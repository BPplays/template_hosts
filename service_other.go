//go:build !windows

package main

import "errors"

func serviceAction(_ *string) error {
	return errors.New("services are only supported on Windows")
}

func srvMain() error {
	return errors.New("services are only supported on Windows")
}
