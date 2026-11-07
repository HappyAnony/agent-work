//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

func execCompat(args []string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return err
	}
	helper := filepath.Join(filepath.Dir(self), "wiki-compat")
	return syscall.Exec(helper, append([]string{helper}, args...), os.Environ())
}
