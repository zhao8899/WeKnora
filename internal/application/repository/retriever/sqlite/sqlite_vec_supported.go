//go:build cgo

package sqlite

import sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"

func sqliteVecEnabled() bool {
	return true
}

func serializeSQLiteVecFloat32(data []float32) ([]byte, error) {
	return sqlite_vec.SerializeFloat32(data)
}
