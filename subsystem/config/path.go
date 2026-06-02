package config

import "flag"

var (
	LocalDir = flag.String("local-dir", "local_data", "本地目录路径，用于存储资源文件")
	// CertFile  证书文件路径，用于HTTPS加密通信
	CertFile = flag.String("cert-file", *LocalDir+"/certs/localhost.pem", "证书文件路径, 用于HTTPS加密通信")
	// KeyFile  私钥文件路径，用于HTTPS加密通信
	KeyFile = flag.String("key-file", *LocalDir+"/certs/localhost-key.pem", "私钥文件路径, 用于HTTPS加密通信")
	// SQLDBPath  SQLite数据库文件路径，统一数据库存储
	SQLDBPath = flag.String("sql-db", *LocalDir+"/database/SQL.db", "SQLite数据库文件路径")
	// VectorDBDir  向量数据库文件夹路径，统一数据库存储
	VectorDBDir = flag.String("vector-db", *LocalDir+"/database/vector", "向量数据库文件夹路径")
)
