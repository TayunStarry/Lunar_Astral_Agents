package forum

// variable.go 论坛引擎层常量与错误值。
//
// 硬截断下限统一 512 字符，避免把服务端的错误详情截得无法定位。

import (
	"errors"
	"sync"
)

const (
	// maxResponseBytes 单次响应体上限，防止异常服务端拖垮进程。
	maxResponseBytes = 8 << 20
	// errorTextRunes 错误详情展示的字符上限。
	errorTextRunes = 512
	// maxTransientRetries forum_deploying 的 GET 重试次数。
	maxTransientRetries = 2

	// AccountSchemaVersion 账号文件结构版本；v2 起移除 model 并新增头像/QQ 脱敏字段。
	AccountSchemaVersion = 2
	// AccountBackupSuffix 账号文件备份后缀；每次覆盖写入前保留上一份，便于回滚。
	AccountBackupSuffix = ".bak"
)

var (
	// ErrEndpointMissing 表示配置里没有可用的论坛服务地址。
	ErrEndpointMissing = errors.New("互助论坛服务地址未配置")
	// ErrMissingAPIKey 表示需要鉴权的接口没有可用 api_key。
	ErrMissingAPIKey = errors.New("互助论坛缺少 api_key")
	// ErrEmptyResponse 表示服务端返回了空响应体。
	ErrEmptyResponse = errors.New("互助论坛返回空响应")

	// ErrAccountPathEmpty 表示账号文件路径为空，无法读写。
	ErrAccountPathEmpty = errors.New("互助论坛账号文件路径为空")
	// ErrAccountNil 表示没有可写入的账号状态。
	ErrAccountNil = errors.New("互助论坛账号状态为空")

	// accountWriteMu 串行化账号文件写入：多个协程同时保存时 .bak 与临时文件不会相互覆盖。
	accountWriteMu sync.Mutex
)
