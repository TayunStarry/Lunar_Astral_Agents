module storage

go 1.24.4

require config v0.0.0

replace config => ../config

require github.com/mattn/go-sqlite3 v1.14.33

require logger v0.0.0

replace logger => ../logger

require github.com/philippgille/chromem-go v0.7.0
