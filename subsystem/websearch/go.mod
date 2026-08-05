module YaraFlow/internal/search

go 1.26

require (
	github.com/chromedp/chromedp v0.16.0
	github.com/mattn/go-sqlite3 v1.14.44
	golang.org/x/net v0.40.0
	storage v0.0.0
)

replace storage => ../storage

replace config => ../config

replace logger => ../logger

require (
	config v0.0.0 // indirect
	github.com/chromedp/cdproto v0.0.0-20260714215040-dc233986426f // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	logger v0.0.0 // indirect
)
