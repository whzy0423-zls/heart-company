package apprelease

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	MaxAPKBytes       int64 = 300 * 1024 * 1024
	MaxMultipartBytes int64 = 301 * 1024 * 1024
)

type CertificateProvider func() (string, error)

type Service struct {
	store       *Store
	files       *FileStore
	inspector   *APKInspector
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
	release, err := s.store.CreateDraft(ctx, Release{Platform: "android", VersionName: info.VersionName, VersionCode: info.VersionCode, ReleaseNotes: notes, FileName: saved.OriginalName, FilePath: saved.Key, FileSize: saved.Size, SHA256: saved.SHA256})
	if err != nil {
		_ = s.files.Remove(saved.Key)
		return Release{}, err
	}
	release.FileAvailable = true
	return release, nil
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
	return s.store.Publish(ctx, id, "android")
}

func (s *Service) Archive(ctx context.Context, id int64) (Release, error) {
	return s.store.Archive(ctx, id, "android")
}
func (s *Service) List(ctx context.Context, page, size int) (ListResult, error) {
	r, err := s.store.List(ctx, page, size)
	if err != nil {
		return r, err
	}
	if r.Current != nil {
		r.Current.FileAvailable = s.fileAvailable(*r.Current)
	}
	for i := range r.Items {
		r.Items[i].FileAvailable = s.fileAvailable(r.Items[i])
	}
	return r, nil
}
func (s *Service) Latest(ctx context.Context, platform string) (Release, error) {
	r, err := s.store.LatestPublished(ctx, platform)
	if err != nil {
		return r, err
	}
	r.FileAvailable = s.fileAvailable(r)
	return r, nil
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
	r.FileAvailable = true
	return r, f, nil
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
func (s *Service) fileAvailable(r Release) bool {
	path, err := s.files.Resolve(r.FilePath)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() == r.FileSize
}
