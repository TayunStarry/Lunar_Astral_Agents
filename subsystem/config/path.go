package config

import "flag"

var (
	LocalDir = flag.String("local-dir", "local_data", "本地目录路径，用于存储资源文件")
	// CertFile  证书文件路径，用于HTTPS加密通信
	CertFile = flag.String("cert-file", *LocalDir+"/certs/localhost.pem", "证书文件路径, 用于HTTPS加密通信")
	// KeyFile  私钥文件路径，用于HTTPS加密通信
	KeyFile = flag.String("key-file", *LocalDir+"/certs/localhost-key.pem", "私钥文件路径, 用于HTTPS加密通信")
	// Database  SQLite数据库文件路径，用于存储系统数据
	Database = flag.String("database", *LocalDir+"/lunar_index.db", "SQLite数据库文件路径, 用于存储系统数据")
)
