//go:build cgo

package container

import sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"

func ensureSQLiteVecAuto() {
	sqlite_vec.Auto()
}
