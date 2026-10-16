//go:build !unix

package app

import (
	"errors"
	"os"
)

func lockStateFile(string) (*os.File, error) {
	return nil, errors.New("durable synchronization state requires POSIX file locking")
}
