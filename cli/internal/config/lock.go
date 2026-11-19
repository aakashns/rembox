package config

import (
	"os"
	"syscall"
)

// lockSSHConfig waits for, then holds, a lock on SSHConfig(dir) until unlock,
// so only one rembox at a time changes it. The lock is on a file beside it, as
// rembox replaces the ssh config rather than writing to it.
func lockSSHConfig(dir string) (unlock func(), err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(SSHConfig(dir)+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil // closing it unlocks it
}
