//go:build windows

package main

import "errors"

func execCompat(args []string) error {
	return errors.New("compatibility helper only supported on macOS/Unix")
}
