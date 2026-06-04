//go:build !windows

package main

import "errors"

func makeService() error {
	return errors.New("services are only supported on Windows")
}
