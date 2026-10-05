//go:build linux || darwin

package workspace

import (
	"golang.org/x/sys/unix"
	"os"
)

func lockFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) }
func syncDirectory(r *os.Root, path string) error {
	f, e := r.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
