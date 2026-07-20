package apprelease

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const randomFileTokenBytes = 16

type FileStore struct {
	root     string
	maxBytes int64
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
	info, err := os.Stat(canonicalRoot)
	if err != nil {
		return nil, fmt.Errorf("stat app release root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("create app release file store: root is not a directory")
	}

	return &FileStore{root: filepath.Clean(canonicalRoot), maxBytes: maxBytes}, nil
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
	if err := s.ensureRootDirectory(); err != nil {
		return StagedFile{}, err
	}

	tmp, err := os.CreateTemp(s.root, ".tmp-*")
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
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return StagedFile{}, fmt.Errorf("close staged APK: %w", err)
	}

	return StagedFile{
		TempPath:     tmpName,
		OriginalName: originalName,
		Size:         n,
		SHA256:       hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func (s *FileStore) Commit(staged StagedFile, platform string, versionCode int64) (SavedFile, error) {
	if platform != "android" {
		return SavedFile{}, ErrUnsupportedPlatform
	}
	if versionCode <= 0 {
		return SavedFile{}, ErrInvalidVersion
	}
	tempPath, err := s.validateStagedPath(staged.TempPath, true)
	if err != nil {
		return SavedFile{}, err
	}

	androidDir := filepath.Join(s.root, "android")
	if err := s.ensureDirectory(androidDir); err != nil {
		return SavedFile{}, fmt.Errorf("create Android release directory: %w", err)
	}

	token, err := randomFileToken()
	if err != nil {
		return SavedFile{}, fmt.Errorf("generate app release filename: %w", err)
	}
	key := path.Join("android", fmt.Sprintf("%d-%s.apk", versionCode, token))
	destination, err := s.Resolve(key)
	if err != nil {
		return SavedFile{}, err
	}
	if _, err := os.Lstat(destination); err == nil {
		return SavedFile{}, fmt.Errorf("commit staged APK: destination already exists")
	} else if !os.IsNotExist(err) {
		return SavedFile{}, fmt.Errorf("check app release destination: %w", err)
	}
	if err := os.Rename(tempPath, destination); err != nil {
		return SavedFile{}, fmt.Errorf("commit staged APK: %w", err)
	}

	return SavedFile{
		Key:          key,
		Path:         destination,
		OriginalName: staged.OriginalName,
		Size:         staged.Size,
		SHA256:       staged.SHA256,
	}, nil
}

func (s *FileStore) Discard(staged StagedFile) error {
	tempPath, err := s.validateStagedPath(staged.TempPath, false)
	if err != nil {
		return err
	}
	info, err := os.Lstat(tempPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat staged APK: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ErrUnsafePath
	}
	if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("discard staged APK: %w", err)
	}
	return nil
}

func (s *FileStore) Remove(key string) error {
	if path.Ext(key) != ".apk" {
		return ErrUnsafePath
	}
	resolved, err := s.Resolve(key)
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
		return fmt.Errorf("remove saved APK: %w", err)
	}
	return nil
}

func (s *FileStore) Resolve(key string) (string, error) {
	cleanKey, err := cleanStorageKey(key)
	if err != nil {
		return "", err
	}
	if err := s.ensureRootDirectory(); err != nil {
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

func (s *FileStore) CleanupStaleTemps(now time.Time, maxAge time.Duration) error {
	if maxAge <= 0 {
		return fmt.Errorf("clean stale app release temps: max age must be positive")
	}
	if err := s.ensureRootDirectory(); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return fmt.Errorf("list staged APK files: %w", err)
	}
	cutoff := now.Add(-maxAge)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".tmp-") {
			continue
		}
		filePath := filepath.Join(s.root, entry.Name())
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
	if err := s.ensureRootDirectory(); err != nil {
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
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".apk" || strings.HasPrefix(entry.Name(), ".tmp-") {
			return nil
		}
		rel, err := filepath.Rel(s.root, filePath)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if !strings.HasPrefix(key, "android/") {
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

func (s *FileStore) ensureRootDirectory() error {
	info, err := os.Lstat(s.root)
	if err != nil {
		return fmt.Errorf("stat app release root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrUnsafePath
	}
	return nil
}

func (s *FileStore) ensureDirectory(directory string) error {
	rel, err := filepath.Rel(s.root, directory)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return ErrUnsafePath
	}
	resolved, err := s.Resolve(filepath.ToSlash(rel))
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
	return nil
}

func (s *FileStore) validateStagedPath(tempPath string, requireExisting bool) (string, error) {
	if tempPath == "" || !filepath.IsAbs(tempPath) {
		return "", ErrUnsafePath
	}
	cleanPath := filepath.Clean(tempPath)
	rel, err := filepath.Rel(s.root, cleanPath)
	if err != nil || rel == "." || filepath.IsAbs(rel) || filepath.Dir(rel) != "." || !strings.HasPrefix(filepath.Base(rel), ".tmp-") {
		return "", ErrUnsafePath
	}
	if err := s.ensureRootDirectory(); err != nil {
		return "", err
	}
	info, err := os.Lstat(cleanPath)
	if os.IsNotExist(err) && !requireExisting {
		return cleanPath, nil
	}
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("stat staged APK: %w", err)
		}
		return "", fmt.Errorf("stat staged APK: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
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
