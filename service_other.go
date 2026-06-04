//go:build !windows

package main

import "errors"

func makeService(_ *string) error {
	return errors.New("services are only supported on Windows")
}

func srvMain() error {
	return errors.New("services are only supported on Windows")
}
