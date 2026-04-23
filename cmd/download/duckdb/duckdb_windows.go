//go:build windows

package main

import "fmt"

func main() {
	panic(fmt.Errorf("duckdb extension download is not supported in this Windows build environment"))
}
