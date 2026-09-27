//go:build dev

package main

import "memolang/internal/devdb"

// Built only with -tags dev (task dev): production binaries do not contain
// the embedded database at all, so a missing DATABASE_URL there can only be
// an error, never a quietly started throwaway database.
func init() {
	startDevDB = func() (string, func(), error) {
		stop, err := devdb.Start()
		return devdb.DSN(), stop, err
	}
}
