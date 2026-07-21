package apprelease

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image/png"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	randomFileTokenBytes = 16
	maxIconBytes         = 8 << 20
)

type FileStore struct {
	root     string
	maxBytes int64

	mu     sync.Mutex
	staged map[string]stagedFileRecord
}

type stagedFileRecord struct {
	path         string
	originalName string
	size         int64
	sha256       string
	info         os.FileInfo
}

func NewFileStore(root string, maxBytes int64) (*FileStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("create app release file store: root is required")
	}
	if maxBytes <= 0 || maxBytes == math.MaxInt64 {
		return nil, fmt.Errorf("create app release file store: invalid maximum size %d", maxBytes)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve app release root: %w", err)
	}
	absRoot = filepath.Clean(absRoot)
	if err := os.MkdirAll(absRoot, 0o750); err != nil {
		return nil, fmt.Errorf("create app release root: %w", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve app release root symlinks: %w", err)
	}
	canonicalRoot, err = filepath.Abs(canonicalRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve canonical app release root: %w", err)
	}
	canonicalRoot = filepath.Clean(canonicalRoot)
	info, err := os.Lstat(canonicalRoot)
	if err != nil {
		return nil, fmt.Errorf("stat app release root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("create app release file store: root is not a directory")
	}
	if err := os.Chmod(canonicalRoot, 0o750); err != nil {
		return nil, fmt.Errorf("set app release root permissions: %w", err)
	}

	return &FileStore{
		root:     canonicalRoot,
		maxBytes: maxBytes,
		staged:   make(map[string]stagedFileRecord),
	}, nil
}

func (s *FileStore) Root() string {
	return s.root
}

func (s *FileStore) Stage(originalName string, src io.Reader) (StagedFile, error) {
	if !strings.EqualFold(filepath.Ext(originalName), ".apk") {
		return StagedFile{}, ErrInvalidExtension
	}
	if src == nil {
		return StagedFile{}, fmt.Errorf("stream staged APK: reader is nil")
	}

	s.mu.Lock()
	if err := s.ensureRootDirectoryLocked(); err != nil {
		s.mu.Unlock()
		return StagedFile{}, err
	}
	tmp, err := os.CreateTemp(s.root, ".tmp-*")
	s.mu.Unlock()
	if err != nil {
		return StagedFile{}, fmt.Errorf("create staged APK: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(0o640); err != nil {
		cleanup()
		return StagedFile{}, fmt.Errorf("set staged APK permissions: %w", err)
	}

	hash := sha256.New()
	limited := io.LimitReader(src, s.maxBytes+1)
	n, copyErr := io.Copy(io.MultiWriter(tmp, hash), limited)
	if copyErr != nil {
		cleanup()
		return StagedFile{}, fmt.Errorf("stream staged APK: %w", copyErr)
	}
	if n > s.maxBytes {
		cleanup()
		return StagedFile{}, ErrFileTooLarge
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return StagedFile{}, fmt.Errorf("sync staged APK: %w", err)
	}
	info, err := tmp.Stat()
	if err != nil {
		cleanup()
		return StagedFile{}, fmt.Errorf("stat staged APK: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != n {
		cleanup()
		return StagedFile{}, ErrStagedFileChanged
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return StagedFile{}, fmt.Errorf("close staged APK: %w", err)
	}

	record := stagedFileRecord{
		path:         tmpName,
		originalName: originalName,
		size:         n,
		sha256:       hex.EncodeToString(hash.Sum(nil)),
		info:         info,
	}
	id, err := s.registerStaged(record)
	if err != nil {
		_ = os.Remove(tmpName)
		return StagedFile{}, err
	}
	return StagedFile{
		id:           id,
		path:         record.path,
		originalName: record.originalName,
		size:         record.size,
		sha256:       record.sha256,
	}, nil
}

func (s *FileStore) Commit(staged StagedFile, platform string, versionCode int64) (SavedFile, error) {
	if platform != "android" {
		return SavedFile{}, ErrUnsupportedPlatform
	}
	if versionCode <= 0 {
		return SavedFile{}, ErrInvalidVersion
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.staged[staged.id]
	if !ok || staged.id == "" {
		return SavedFile{}, ErrUnsafePath
	}

	androidDir := filepath.Join(s.root, "android")
	if err := s.ensureDirectoryLocked(androidDir); err != nil {
		return SavedFile{}, fmt.Errorf("create Android release directory: %w", err)
	}

	stagedFile, stagedInfo, err := s.openRegisteredStagedLocked(record)
	if err != nil {
		return SavedFile{}, err
	}
	closed := false
	defer func() {
		if !closed {
			_ = stagedFile.Close()
		}
	}()

	hash := sha256.New()
	n, err := io.Copy(hash, stagedFile)
	if err != nil {
		return SavedFile{}, fmt.Errorf("rehash staged APK: %w", err)
	}
	afterHashInfo, err := stagedFile.Stat()
	if err != nil {
		return SavedFile{}, fmt.Errorf("stat rehashed staged APK: %w", err)
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if !sameRegisteredFile(record, stagedInfo) ||
		!sameRegisteredFile(record, afterHashInfo) ||
		!os.SameFile(stagedInfo, afterHashInfo) ||
		n != record.size || digest != record.sha256 {
		return SavedFile{}, ErrStagedFileChanged
	}

	pathInfo, err := os.Lstat(record.path)
	if err != nil {
		if os.IsNotExist(err) {
			return SavedFile{}, ErrStagedFileChanged
		}
		return SavedFile{}, fmt.Errorf("recheck staged APK path: %w", err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() || !os.SameFile(afterHashInfo, pathInfo) {
		return SavedFile{}, ErrStagedFileChanged
	}

	token, err := randomFileToken()
	if err != nil {
		return SavedFile{}, fmt.Errorf("generate app release filename: %w", err)
	}
	key := path.Join("android", fmt.Sprintf("%d-%s.apk", versionCode, token))
	destination, err := s.resolveLocked(key)
	if err != nil {
		return SavedFile{}, err
	}
	if _, err := os.Lstat(destination); err == nil {
		return SavedFile{}, fmt.Errorf("commit staged APK: destination already exists")
	} else if !os.IsNotExist(err) {
		return SavedFile{}, fmt.Errorf("check app release destination: %w", err)
	}
	if err := os.Rename(record.path, destination); err != nil {
		return SavedFile{}, fmt.Errorf("commit staged APK: %w", err)
	}

	if err := s.verifyCommittedFileLocked(stagedFile, record, key, destination); err != nil {
		return SavedFile{}, s.rollbackCommittedFileLocked(staged.id, destination, err)
	}
	if err := syncDirectory(androidDir); err != nil {
		return SavedFile{}, s.rollbackCommittedFileLocked(staged.id, destination, fmt.Errorf("sync Android release directory: %w", err))
	}
	if err := stagedFile.Close(); err != nil {
		closed = true
		return SavedFile{}, s.rollbackCommittedFileLocked(staged.id, destination, fmt.Errorf("close committed APK: %w", err))
	}
	closed = true
	delete(s.staged, staged.id)

	return SavedFile{
		Key:          key,
		Path:         destination,
		OriginalName: record.originalName,
		Size:         record.size,
		SHA256:       record.sha256,
	}, nil
}

func (s *FileStore) Discard(staged StagedFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.staged[staged.id]
	if !ok || staged.id == "" {
		return nil
	}
	if err := s.ensureRootDirectoryLocked(); err != nil {
		return err
	}
	info, err := os.Lstat(record.path)
	if os.IsNotExist(err) {
		delete(s.staged, staged.id)
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat staged APK: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !os.SameFile(record.info, info) {
		delete(s.staged, staged.id)
		return ErrStagedFileChanged
	}
	if err := os.Remove(record.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("discard staged APK: %w", err)
	}
	delete(s.staged, staged.id)
	return nil
}

func (s *FileStore) SaveIcon(apkKey string, pngData []byte) (string, error) {
	if err := validateManagedArtifactKey(apkKey, ".apk"); err != nil {
		return "", err
	}
	if len(pngData) > maxIconBytes {
		return "", ErrFileTooLarge
	}
	if len(pngData) == 0 {
		return "", fmt.Errorf("save app release icon: PNG data is empty")
	}
	config, err := png.DecodeConfig(bytes.NewReader(pngData))
	if err != nil {
		return "", fmt.Errorf("save app release icon: invalid PNG: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 ||
		config.Width > maxAPKIconDimension || config.Height > maxAPKIconDimension ||
		int64(config.Width)*int64(config.Height) > maxAPKIconPixels {
		return "", fmt.Errorf("save app release icon: invalid PNG dimensions")
	}
	if _, err := png.Decode(bytes.NewReader(pngData)); err != nil {
		return "", fmt.Errorf("save app release icon: invalid PNG: %w", err)
	}

	iconKey := strings.TrimSuffix(apkKey, ".apk") + ".png"
	s.mu.Lock()
	defer s.mu.Unlock()
	destination, err := s.resolveLocked(iconKey)
	if err != nil {
		return "", err
	}
	iconDir := filepath.Dir(destination)
	if err := s.ensureDirectoryTreeLocked(iconDir); err != nil {
		return "", fmt.Errorf("create Android release icon directory: %w", err)
	}
	if info, err := os.Lstat(destination); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", ErrUnsafePath
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check app release icon destination: %w", err)
	}

	tmp, err := os.CreateTemp(iconDir, "."+filepath.Base(destination)+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("create app release icon temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := tmp.Chmod(0o640); err != nil {
		cleanup()
		return "", fmt.Errorf("set app release icon permissions: %w", err)
	}
	n, err := tmp.Write(pngData)
	if err != nil {
		cleanup()
		return "", fmt.Errorf("write app release icon: %w", err)
	}
	if n != len(pngData) {
		cleanup()
		return "", io.ErrShortWrite
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return "", fmt.Errorf("sync app release icon: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("close app release icon: %w", err)
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("commit app release icon: %w", err)
	}
	info, err := os.Lstat(destination)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() != int64(len(pngData)) {
		_ = os.Remove(destination)
		if err != nil {
			return "", fmt.Errorf("verify app release icon: %w", err)
		}
		return "", ErrUnsafePath
	}
	if err := syncDirectory(iconDir); err != nil {
		_ = os.Remove(destination)
		return "", fmt.Errorf("sync Android release directory: %w", err)
	}
	return iconKey, nil
}

func (s *FileStore) Remove(key string) error {
	if err := validateManagedArtifactKey(key, ".apk", ".png"); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	resolved, err := s.resolveLocked(key)
	if err != nil {
		return err
	}
	info, err := os.Lstat(resolved)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat saved APK: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ErrUnsafePath
	}
	if err := os.Remove(resolved); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove saved app release file: %w", err)
	}
	if err := syncDirectory(filepath.Dir(resolved)); err != nil {
		return fmt.Errorf("sync app release directory after remove: %w", err)
	}
	return nil
}

func (s *FileStore) Resolve(key string) (string, error) {
	if err := validateManagedArtifactKey(key, ".apk", ".png"); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resolveLocked(key)
}

func (s *FileStore) CleanupStaleTemps(now time.Time, maxAge time.Duration) error {
	if maxAge <= 0 {
		return fmt.Errorf("clean stale app release temps: max age must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureRootDirectoryLocked(); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return fmt.Errorf("list staged APK files: %w", err)
	}
	registered := make(map[string]struct{}, len(s.staged))
	for _, record := range s.staged {
		registered[record.path] = struct{}{}
	}
	cutoff := now.Add(-maxAge)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".tmp-") {
			continue
		}
		filePath := filepath.Join(s.root, entry.Name())
		if _, ok := registered[filePath]; ok {
			continue
		}
		info, err := os.Lstat(filePath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("stat staged APK %q: %w", entry.Name(), err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale staged APK %q: %w", entry.Name(), err)
		}
	}
	return nil
}

func (s *FileStore) AuditOrphans(referenced map[string]struct{}) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureRootDirectoryLocked(); err != nil {
		return nil, err
	}
	orphans := make([]string, 0)
	err := filepath.WalkDir(s.root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filePath == s.root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() || strings.HasPrefix(entry.Name(), ".tmp-") {
			return nil
		}
		rel, err := filepath.Rel(s.root, filePath)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if validateManagedArtifactKey(key, ".apk", ".png") != nil {
			return nil
		}
		if _, ok := referenced[key]; !ok {
			orphans = append(orphans, key)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("audit app release files: %w", err)
	}
	sort.Strings(orphans)
	return orphans, nil
}

func validateManagedArtifactKey(key string, extensions ...string) error {
	cleaned, err := cleanStorageKey(key)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(cleaned, "android/") || filepath.Base(cleaned) == "." {
		return ErrUnsafePath
	}
	ext := path.Ext(cleaned)
	for _, allowed := range extensions {
		if ext == allowed {
			return nil
		}
	}
	return ErrUnsafePath
}

func (s *FileStore) registerStaged(record stagedFileRecord) (string, error) {
	for {
		id, err := randomFileToken()
		if err != nil {
			return "", fmt.Errorf("generate staged APK handle: %w", err)
		}
		s.mu.Lock()
		if _, exists := s.staged[id]; exists {
			s.mu.Unlock()
			continue
		}
		if err := s.ensureRootDirectoryLocked(); err != nil {
			s.mu.Unlock()
			return "", err
		}
		cleanPath, err := s.validateStagedPathLocked(record.path)
		if err != nil {
			s.mu.Unlock()
			return "", err
		}
		info, err := os.Lstat(cleanPath)
		if err != nil {
			s.mu.Unlock()
			return "", fmt.Errorf("register staged APK: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !os.SameFile(record.info, info) {
			s.mu.Unlock()
			return "", ErrStagedFileChanged
		}
		s.staged[id] = record
		s.mu.Unlock()
		return id, nil
	}
}

func (s *FileStore) openRegisteredStagedLocked(record stagedFileRecord) (*os.File, os.FileInfo, error) {
	if err := s.ensureRootDirectoryLocked(); err != nil {
		return nil, nil, err
	}
	cleanPath, err := s.validateStagedPathLocked(record.path)
	if err != nil {
		return nil, nil, err
	}
	pathInfo, err := os.Lstat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, ErrStagedFileChanged
		}
		return nil, nil, fmt.Errorf("stat staged APK: %w", err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() || !os.SameFile(record.info, pathInfo) {
		return nil, nil, ErrStagedFileChanged
	}
	file, err := os.Open(cleanPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open staged APK: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, fmt.Errorf("stat opened staged APK: %w", err)
	}
	if !sameRegisteredFile(record, info) || !os.SameFile(pathInfo, info) {
		_ = file.Close()
		return nil, nil, ErrStagedFileChanged
	}
	return file, info, nil
}

func (s *FileStore) verifyCommittedFileLocked(file *os.File, record stagedFileRecord, key, destination string) error {
	resolved, err := s.resolveLocked(key)
	if err != nil {
		return err
	}
	if resolved != destination {
		return ErrUnsafePath
	}
	destinationInfo, err := os.Lstat(resolved)
	if err != nil {
		return fmt.Errorf("lstat committed APK: %w", err)
	}
	if destinationInfo.Mode()&os.ModeSymlink != 0 || !destinationInfo.Mode().IsRegular() {
		return ErrUnsafePath
	}
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat committed APK descriptor: %w", err)
	}
	if !sameRegisteredFile(record, fileInfo) || !os.SameFile(fileInfo, destinationInfo) {
		return ErrStagedFileChanged
	}
	return nil
}

func (s *FileStore) rollbackCommittedFileLocked(id, destination string, cause error) error {
	delete(s.staged, id)
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w (cleanup committed APK: %v)", cause, err)
	}
	return cause
}

func sameRegisteredFile(record stagedFileRecord, info os.FileInfo) bool {
	return info != nil &&
		info.Mode().IsRegular() &&
		os.SameFile(record.info, info) &&
		info.Mode() == record.info.Mode() &&
		info.Size() == record.size &&
		info.ModTime().Equal(record.info.ModTime())
}

func (s *FileStore) resolveLocked(key string) (string, error) {
	cleanKey, err := cleanStorageKey(key)
	if err != nil {
		return "", err
	}
	if err := s.ensureRootDirectoryLocked(); err != nil {
		return "", err
	}

	candidate := filepath.Join(s.root, filepath.FromSlash(cleanKey))
	rel, err := filepath.Rel(s.root, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve app release key: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", ErrUnsafePath
	}

	current := s.root
	parts := strings.Split(cleanKey, "/")
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			break
		}
		if statErr != nil {
			return "", fmt.Errorf("inspect app release path: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", ErrUnsafePath
		}
		if i < len(parts)-1 && !info.IsDir() {
			return "", ErrUnsafePath
		}
	}

	return candidate, nil
}

func (s *FileStore) ensureRootDirectoryLocked() error {
	info, err := os.Lstat(s.root)
	if err != nil {
		return fmt.Errorf("stat app release root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrUnsafePath
	}
	return nil
}

func (s *FileStore) ensureDirectoryLocked(directory string) error {
	rel, err := filepath.Rel(s.root, directory)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return ErrUnsafePath
	}
	resolved, err := s.resolveLocked(filepath.ToSlash(rel))
	if err != nil {
		return err
	}
	if resolved != filepath.Clean(directory) {
		return ErrUnsafePath
	}
	info, err := os.Lstat(directory)
	if os.IsNotExist(err) {
		if err := os.Mkdir(directory, 0o750); err != nil && !os.IsExist(err) {
			return err
		}
		info, err = os.Lstat(directory)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrUnsafePath
	}
	if err := os.Chmod(directory, 0o750); err != nil {
		return err
	}
	info, err = os.Lstat(directory)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm() != 0o750 {
		return ErrUnsafePath
	}
	return nil
}

func (s *FileStore) ensureDirectoryTreeLocked(directory string) error {
	rel, err := filepath.Rel(s.root, directory)
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return ErrUnsafePath
	}
	current := s.root
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." || part == ".." {
			return ErrUnsafePath
		}
		current = filepath.Join(current, part)
		if err := s.ensureDirectoryLocked(current); err != nil {
			return err
		}
	}
	return nil
}

func (s *FileStore) validateStagedPathLocked(tempPath string) (string, error) {
	if tempPath == "" || !filepath.IsAbs(tempPath) {
		return "", ErrUnsafePath
	}
	cleanPath := filepath.Clean(tempPath)
	rel, err := filepath.Rel(s.root, cleanPath)
	if err != nil || rel == "." || filepath.IsAbs(rel) || filepath.Dir(rel) != "." || !strings.HasPrefix(filepath.Base(rel), ".tmp-") {
		return "", ErrUnsafePath
	}
	return cleanPath, nil
}

func cleanStorageKey(key string) (string, error) {
	if key == "" || strings.ContainsRune(key, '\x00') || strings.Contains(key, `\`) {
		return "", ErrUnsafePath
	}
	if filepath.IsAbs(key) || path.IsAbs(key) || filepath.VolumeName(key) != "" || looksLikeWindowsDrivePath(key) {
		return "", ErrUnsafePath
	}
	cleaned := path.Clean(key)
	if cleaned == "." || cleaned != key {
		return "", ErrUnsafePath
	}
	for _, part := range strings.Split(cleaned, "/") {
		if part == "" || part == "." || part == ".." {
			return "", ErrUnsafePath
		}
	}
	return cleaned, nil
}

func looksLikeWindowsDrivePath(key string) bool {
	if len(key) < 2 || key[1] != ':' {
		return false
	}
	first := key[0]
	return first >= 'a' && first <= 'z' || first >= 'A' && first <= 'Z'
}

func randomFileToken() (string, error) {
	raw := make([]byte, randomFileTokenBytes)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func syncDirectory(directory string) error {
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return err
	}
	return dir.Close()
}
