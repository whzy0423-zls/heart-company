//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package apprelease

import "fmt"

func (s *FileStore) saveIconAtomic(string, string, []byte) error {
	return fmt.Errorf("%w: app release icon storage is unavailable on this platform", ErrUnsupportedPlatform)
}
