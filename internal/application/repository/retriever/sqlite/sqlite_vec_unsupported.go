//go:build !cgo

package sqlite

import "fmt"

func sqliteVecEnabled() bool {
	return false
}

func serializeSQLiteVecFloat32(_ []float32) ([]byte, error) {
	return nil, fmt.Errorf("sqlite-vec requires a CGO-enabled build")
}
