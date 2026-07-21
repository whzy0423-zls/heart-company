//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package apprelease

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func (s *FileStore) saveIconAtomic(iconDir, destinationName string, pngData []byte) error {
	directory, directoryInfo, err := openDirectoryNoFollow(iconDir)
	if err != nil {
		return err
	}
	defer directory.Close()
	dirfd := int(directory.Fd())
	if err := requireMissingAt(dirfd, destinationName); err != nil {
		return err
	}

	tmp, tempName, tempStat, err := createIconTempAt(dirfd, destinationName)
	if err != nil {
		return err
	}
	tmpPath := filepath.Join(iconDir, tempName)
	destinationPath := filepath.Join(iconDir, destinationName)
	cleanupTemp := func() {
		_ = tmp.Close()
		unlinkatIfSameFile(dirfd, tempName, tempStat)
	}
	if s.afterIconTempCreated != nil {
		s.afterIconTempCreated(tmpPath)
	}
	if err := validateDirectoryIdentity(iconDir, directory, directoryInfo); err != nil {
		cleanupTemp()
		return err
	}
	if !pathAtMatchesFile(dirfd, tempName, tempStat) {
		cleanupTemp()
		return ErrUnsafePath
	}
	if err := tmp.Chmod(0o640); err != nil {
		cleanupTemp()
		return fmt.Errorf("set app release icon permissions: %w", err)
	}
	n, err := tmp.Write(pngData)
	if err != nil {
		cleanupTemp()
		return fmt.Errorf("write app release icon: %w", err)
	}
	if n != len(pngData) {
		cleanupTemp()
		return io.ErrShortWrite
	}
	if err := tmp.Sync(); err != nil {
		cleanupTemp()
		return fmt.Errorf("sync app release icon: %w", err)
	}
	if err := tmp.Close(); err != nil {
		unlinkatIfSameFile(dirfd, tempName, tempStat)
		return fmt.Errorf("close app release icon: %w", err)
	}
	if err := validateDirectoryIdentity(iconDir, directory, directoryInfo); err != nil {
		unlinkatIfSameFile(dirfd, tempName, tempStat)
		return err
	}
	if !pathAtMatchesFile(dirfd, tempName, tempStat) {
		return ErrUnsafePath
	}
	if s.beforeIconCommit != nil {
		s.beforeIconCommit(tmpPath, destinationPath)
	}
	if err := unix.Linkat(dirfd, tempName, dirfd, destinationName, 0); err != nil {
		unlinkatIfSameFile(dirfd, tempName, tempStat)
		if errors.Is(err, unix.EEXIST) {
			return fmt.Errorf("save app release icon: destination already exists")
		}
		return fmt.Errorf("commit app release icon: %w", err)
	}
	if !pathAtMatchesFile(dirfd, destinationName, tempStat) {
		unlinkatIfSameFile(dirfd, destinationName, tempStat)
		unlinkatIfSameFile(dirfd, tempName, tempStat)
		return ErrUnsafePath
	}
	if err := unix.Unlinkat(dirfd, tempName, 0); err != nil {
		unlinkatIfSameFile(dirfd, destinationName, tempStat)
		unlinkatIfSameFile(dirfd, tempName, tempStat)
		return fmt.Errorf("remove committed app release icon temp: %w", err)
	}
	if err := validateDirectoryIdentity(iconDir, directory, directoryInfo); err != nil {
		unlinkatIfSameFile(dirfd, destinationName, tempStat)
		return err
	}
	if !pathAtMatchesFile(dirfd, destinationName, tempStat) {
		return ErrUnsafePath
	}
	destinationStat, err := statAtNoFollow(dirfd, destinationName)
	if err != nil || destinationStat.Size != int64(len(pngData)) {
		unlinkatIfSameFile(dirfd, destinationName, tempStat)
		if err != nil {
			return fmt.Errorf("verify app release icon: %w", err)
		}
		return ErrUnsafePath
	}
	if err := validateDirectoryIdentity(iconDir, directory, directoryInfo); err != nil {
		unlinkatIfSameFile(dirfd, destinationName, tempStat)
		return err
	}
	if err := unix.Fsync(dirfd); err != nil {
		unlinkatIfSameFile(dirfd, destinationName, tempStat)
		return fmt.Errorf("sync Android release directory: %w", err)
	}
	if err := validateDirectoryIdentity(iconDir, directory, directoryInfo); err != nil {
		unlinkatIfSameFile(dirfd, destinationName, tempStat)
		return err
	}
	if !pathAtMatchesFile(dirfd, destinationName, tempStat) {
		return ErrUnsafePath
	}
	return nil
}

func openDirectoryNoFollow(directory string) (*os.File, os.FileInfo, error) {
	pathInfo, err := os.Lstat(directory)
	if err != nil {
		return nil, nil, fmt.Errorf("stat app release icon directory: %w", err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.IsDir() {
		return nil, nil, ErrUnsafePath
	}
	fd, err := unix.Open(directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open app release icon directory: %w", err)
	}
	dir := os.NewFile(uintptr(fd), directory)
	if dir == nil {
		_ = unix.Close(fd)
		return nil, nil, fmt.Errorf("open app release icon directory: invalid descriptor")
	}
	descriptorInfo, err := dir.Stat()
	if err != nil {
		_ = dir.Close()
		return nil, nil, fmt.Errorf("stat opened app release icon directory: %w", err)
	}
	if !descriptorInfo.IsDir() || !os.SameFile(pathInfo, descriptorInfo) {
		_ = dir.Close()
		return nil, nil, ErrUnsafePath
	}
	return dir, descriptorInfo, nil
}

func validateDirectoryIdentity(directory string, descriptor *os.File, expected os.FileInfo) error {
	pathInfo, err := os.Lstat(directory)
	if err != nil {
		return ErrUnsafePath
	}
	descriptorInfo, err := descriptor.Stat()
	if err != nil {
		return ErrUnsafePath
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.IsDir() || !descriptorInfo.IsDir() ||
		!os.SameFile(expected, pathInfo) || !os.SameFile(expected, descriptorInfo) {
		return ErrUnsafePath
	}
	return nil
}

func createIconTempAt(dirfd int, destinationName string) (*os.File, string, unix.Stat_t, error) {
	for attempts := 0; attempts < 100; attempts++ {
		token, err := randomFileToken()
		if err != nil {
			return nil, "", unix.Stat_t{}, fmt.Errorf("generate app release icon temp name: %w", err)
		}
		name := "." + destinationName + ".tmp-" + token
		fd, err := unix.Openat(dirfd, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o640)
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		if err != nil {
			return nil, "", unix.Stat_t{}, fmt.Errorf("create app release icon temp: %w", err)
		}
		file := os.NewFile(uintptr(fd), name)
		if file == nil {
			_ = unix.Close(fd)
			return nil, "", unix.Stat_t{}, fmt.Errorf("create app release icon temp: invalid descriptor")
		}
		var stat unix.Stat_t
		if err := unix.Fstat(fd, &stat); err != nil {
			_ = file.Close()
			return nil, "", unix.Stat_t{}, fmt.Errorf("stat app release icon temp: %w", err)
		}
		if !unixStatIsRegular(stat) {
			_ = file.Close()
			return nil, "", unix.Stat_t{}, ErrUnsafePath
		}
		return file, name, stat, nil
	}
	return nil, "", unix.Stat_t{}, fmt.Errorf("create app release icon temp: too many name collisions")
}

func requireMissingAt(dirfd int, name string) error {
	stat, err := statAtNoFollow(dirfd, name)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check app release icon destination: %w", err)
	}
	if !unixStatIsRegular(stat) {
		return ErrUnsafePath
	}
	return fmt.Errorf("save app release icon: destination already exists")
}

func statAtNoFollow(dirfd int, name string) (unix.Stat_t, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(dirfd, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	return stat, err
}

func pathAtMatchesFile(dirfd int, name string, expected unix.Stat_t) bool {
	stat, err := statAtNoFollow(dirfd, name)
	return err == nil && unixStatIsRegular(stat) && sameUnixFile(stat, expected)
}

func unlinkatIfSameFile(dirfd int, name string, expected unix.Stat_t) {
	if pathAtMatchesFile(dirfd, name, expected) {
		_ = unix.Unlinkat(dirfd, name, 0)
	}
}

func unixStatIsRegular(stat unix.Stat_t) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFREG
}

func sameUnixFile(left, right unix.Stat_t) bool {
	return left.Dev == right.Dev && left.Ino == right.Ino
}
