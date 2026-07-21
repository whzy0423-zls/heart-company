package apprelease

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	MaxAPKBytes       int64 = 300 * 1024 * 1024
	MaxMultipartBytes int64 = 301 * 1024 * 1024
)

type CertificateProvider func() (string, error)

type releaseStore interface {
	CreateDraft(context.Context, Release) (Release, error)
	FindByID(context.Context, int64) (Release, error)
	Publish(context.Context, int64, string) (Release, error)
	Archive(context.Context, int64, string) (Release, error)
	List(context.Context, int, int) (ListResult, error)
	LatestPublished(context.Context, string) (Release, error)
	ReferencedKeys(context.Context) (map[string]struct{}, error)
}

type apkInspector interface {
	Inspect(string) (APKInfo, error)
}

type Service struct {
	store       releaseStore
	files       *FileStore
	inspector   apkInspector
	packageName string
	certificate CertificateProvider
}

func NewService(store *Store, files *FileStore, inspector *APKInspector, packageName string, certificate CertificateProvider) *Service {
	return &Service{store: store, files: files, inspector: inspector, packageName: packageName, certificate: certificate}
}

func (s *Service) StageAPK(name string, src io.Reader) (StagedFile, error) {
	return s.files.Stage(name, src)
}
func (s *Service) DiscardStaged(file StagedFile) error { return s.files.Discard(file) }

func (s *Service) CreateDraftFromStaged(ctx context.Context, staged StagedFile, notes string) (Release, error) {
	info, err := s.inspector.Inspect(staged.Path())
	if err != nil {
		_ = s.files.Discard(staged)
		return Release{}, err
	}
	if err := ValidateUploadAPK(info, s.packageName); err != nil {
		_ = s.files.Discard(staged)
		return Release{}, err
	}
	saved, err := s.files.Commit(staged, "android", info.VersionCode)
	if err != nil {
		_ = s.files.Discard(staged)
		return Release{}, err
	}
	iconKey := ""
	if len(info.IconPNG) > 0 {
		if savedIconKey, saveErr := s.files.SaveIcon(saved.Key, info.IconPNG); saveErr == nil {
			iconKey = savedIconKey
		}
	}
	release, err := s.store.CreateDraft(ctx, Release{
		Platform:     "android",
		AppName:      info.AppName,
		PackageName:  info.PackageName,
		IconPath:     iconKey,
		VersionName:  info.VersionName,
		VersionCode:  info.VersionCode,
		ReleaseNotes: notes,
		FileName:     saved.OriginalName,
		FilePath:     saved.Key,
		FileSize:     saved.Size,
		SHA256:       saved.SHA256,
	})
	if err != nil {
		if iconKey != "" {
			_ = s.files.Remove(iconKey)
		}
		_ = s.files.Remove(saved.Key)
		return Release{}, err
	}
	return s.enrichRelease(release), nil
}

func (s *Service) Publish(ctx context.Context, id int64) (Release, error) {
	release, err := s.store.FindByID(ctx, id)
	if err != nil {
		return Release{}, err
	}
	path, err := s.files.Resolve(release.FilePath)
	if err != nil {
		return Release{}, err
	}
	info, err := s.inspector.Inspect(path)
	if err != nil {
		return Release{}, err
	}
	cert, err := s.certificate()
	if err != nil {
		return Release{}, ErrPublishCertificateUnavailable
	}
	if err := ValidateUploadAPK(info, s.packageName); err != nil {
		return Release{}, err
	}
	if err := ValidatePublishAPK(info, cert); err != nil {
		return Release{}, err
	}
	published, err := s.store.Publish(ctx, id, "android")
	if err != nil {
		return Release{}, err
	}
	return s.enrichRelease(published), nil
}

func (s *Service) Archive(ctx context.Context, id int64) (Release, error) {
	archived, err := s.store.Archive(ctx, id, "android")
	if err != nil {
		return Release{}, err
	}
	return s.enrichRelease(archived), nil
}
func (s *Service) List(ctx context.Context, page, size int) (ListResult, error) {
	r, err := s.store.List(ctx, page, size)
	if err != nil {
		return r, err
	}
	if r.Current != nil {
		enriched := s.enrichRelease(*r.Current)
		r.Current = &enriched
	}
	for i := range r.Items {
		r.Items[i] = s.enrichRelease(r.Items[i])
	}
	return r, nil
}
func (s *Service) Latest(ctx context.Context, platform string) (Release, error) {
	r, err := s.store.LatestPublished(ctx, platform)
	if err != nil {
		return r, err
	}
	return s.enrichRelease(r), nil
}
func (s *Service) Open(ctx context.Context, id int64) (Release, *os.File, error) {
	r, err := s.store.FindByID(ctx, id)
	if err != nil {
		return r, nil, err
	}
	path, err := s.files.Resolve(r.FilePath)
	if err != nil {
		return r, nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return r, nil, err
	}
	return s.enrichRelease(r), f, nil
}

func (s *Service) OpenIcon(ctx context.Context, id int64) (Release, *os.File, error) {
	r, err := s.store.FindByID(ctx, id)
	if err != nil {
		return r, nil, err
	}
	if strings.TrimSpace(r.IconPath) == "" {
		return r, nil, ErrNotFound
	}
	if err := validateManagedArtifactKey(r.IconPath, ".png"); err != nil {
		return r, nil, err
	}
	iconPath, err := s.files.Resolve(r.IconPath)
	if err != nil {
		return r, nil, err
	}
	file, err := os.Open(iconPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return r, nil, ErrNotFound
		}
		return r, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return r, nil, err
	}
	pathInfo, err := os.Lstat(iconPath)
	if err != nil {
		_ = file.Close()
		if errors.Is(err, os.ErrNotExist) {
			return r, nil, ErrNotFound
		}
		return r, nil, err
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(pathInfo, info) {
		_ = file.Close()
		return r, nil, ErrUnsafePath
	}
	if !pathInfo.Mode().IsRegular() || !info.Mode().IsRegular() {
		_ = file.Close()
		return r, nil, ErrNotFound
	}
	return s.enrichRelease(r), file, nil
}

func (s *Service) Maintain(ctx context.Context, now time.Time) error {
	if err := s.files.CleanupStaleTemps(now, 24*time.Hour); err != nil {
		return err
	}
	keys, err := s.store.ReferencedKeys(ctx)
	if err != nil {
		return err
	}
	orphans, err := s.files.AuditOrphans(keys)
	if err != nil {
		return err
	}
	if len(orphans) > 0 {
		return fmt.Errorf("app release orphan files: %v", orphans)
	}
	return nil
}

func (s *Service) enrichRelease(r Release) Release {
	r.FileAvailable = s.fileAvailable(r)
	r.IconURL = ""
	if r.ID > 0 && s.iconAvailable(r) {
		r.IconURL = fmt.Sprintf("/api/app-release-icons/%d", r.ID)
	}
	return r
}

func (s *Service) fileAvailable(r Release) bool {
	if validateManagedArtifactKey(r.FilePath, ".apk") != nil {
		return false
	}
	path, err := s.files.Resolve(r.FilePath)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() == r.FileSize
}

func (s *Service) iconAvailable(r Release) bool {
	if validateManagedArtifactKey(r.IconPath, ".png") != nil {
		return false
	}
	path, err := s.files.Resolve(r.IconPath)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
