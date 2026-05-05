module storage

go 1.24.4

require config v0.0.0

replace config => ../config

require (
	github.com/mattn/go-sqlite3 v1.14.33
)
