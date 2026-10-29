package db

import "io/fs"

// mustSub membuka subfolder migrations dari embed; panic hanya mungkin bila folder hilang saat build.
func mustSub() fs.FS {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}
