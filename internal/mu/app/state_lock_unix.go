//go:build unix

package app

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func lockStateFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, errors.New("synchronization state is already locked or file locking is unsupported")
	}
	return file, nil
}
