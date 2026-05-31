package module

import (
	"sync"

	chromem "github.com/philippgille/chromem-go"
)

// db 是 chromem 数据库实例
var db *chromem.DB

// collection 是 chromem 数据库中的集合实例
var collection *chromem.Collection

// initOnce 是初始化一次的 sync.Once 实例
var initOnce sync.Once

// initErr 是初始化 chromem 数据库时的错误
var initErr error

// messageIDCounter 是消息ID计数器 用于生成唯一的消息ID
var messageIDCounter int

// documentEntries 内存中的文档列表 — 按插入顺序排列，用于前端分页浏览
var documentEntries []DocumentEntry

// documentEntriesMu 保护 documentEntries 的读写锁
var documentEntriesMu sync.RWMutex

// entriesFilePath entries.json 持久化文件路径
var entriesFilePath string
