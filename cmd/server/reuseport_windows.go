//go:build windows

package main

func setReusePort(_ uintptr) error {
	return nil
}
