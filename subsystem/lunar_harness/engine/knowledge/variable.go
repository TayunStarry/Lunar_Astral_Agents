package knowledge

// variable.go 全部全局变量与常量声明集中于此。

import (
	"database/sql"
	"regexp"
	"sync"

	"LunarSubsystem/LunarHarness/engine/config"
)

// kvTable 生成 key/value 知识表建表语句（表名仅接受内部常量）。
func kvTable(name string) string {
	return `CREATE TABLE IF NOT EXISTS ` + name + ` (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	)`
}

var (
	// store 通用知识库单例（懒初始化）
	store = &Store{}

	// mediaDB 媒体派生数据：媒体理解摘要 + 视觉副本簿记。
	mediaDB = &domainDB{
		label: "媒体派生",
		path:  func() string { return config.MediaDBPath },
		schema: []string{
			kvTable("video_summary"),
			`CREATE TABLE IF NOT EXISTS vision_cache (
				content_hash TEXT NOT NULL,
				side         INTEGER NOT NULL,
				tag          TEXT NOT NULL DEFAULT '',
				cache_path   TEXT NOT NULL DEFAULT '',
				bytes        INTEGER NOT NULL DEFAULT 0,
				created_at   INTEGER NOT NULL DEFAULT 0,
				used_at      INTEGER NOT NULL DEFAULT 0,
				PRIMARY KEY (content_hash, side)
			)`,
		},
	}
	// webDB 网络检索缓存：页面摘要。
	webDB = &domainDB{
		label: "网络检索",
		path:  func() string { return config.WebDBPath },
		schema: []string{
			`CREATE TABLE IF NOT EXISTS web_ltp_page_cache (
				url        TEXT PRIMARY KEY,
				title      TEXT NOT NULL DEFAULT '',
				domain     TEXT NOT NULL DEFAULT '',
				summary    TEXT NOT NULL DEFAULT '',
				query_hint TEXT NOT NULL DEFAULT '',
				fetched_at INTEGER NOT NULL
			)`,
		},
	}
	// textDB 文本内容索引：文件标识 → 分块集合。
	textDB = &domainDB{
		label:  "文本索引",
		path:   func() string { return config.TextDBPath },
		schema: []string{kvTable("file_mapping")},
	}
	// characterDB 自定义角色库：绘图角色档案。
	characterDB = &domainDB{
		label:  "自定义角色",
		path:   func() string { return config.CharacterDBPath },
		schema: []string{kvTable("painter_characters")},
	}

	// domains 内建领域库清单（初始化 / 关闭统一遍历）。
	domains = []*domainDB{mediaDB, webDB, textDB, characterDB}

	// domainOpeners 已登记的域库打开函数（RegisterDomain 注册，name 为 ExecuteOn 的 db 参数）。
	domainOpeners = map[string]func() (*sql.DB, error){}
	// domainMu 保护 domainOpeners 的并发读写。
	domainMu sync.RWMutex

	// reservedDomains 宿主内建域库名：未登记时 ExecuteOn 明确报错，不回退为 database 内的库文件。
	reservedDomains = map[string]bool{
		"archive":   true,
		"media":     true,
		"web":       true,
		"text":      true,
		"character": true,
	}

	// knowledgeBuiltinTables 内建逻辑表 → 实际落库的领域库。
	// 未登记的表仍落通用知识库 knowledge.db（用户 / 插件自建表）。
	knowledgeBuiltinTables = map[string]*domainDB{
		"video_summary":      mediaDB,     // 动图 / 视频 / 音频理解摘要
		"file_mapping":       textDB,      // 文本文件 → 分块集合索引
		"painter_characters": characterDB, // 自定义角色档案
	}

	// legacyKnowledgeTables 迁移清单；列顺序必须与目标表一致。
	legacyKnowledgeTables = []legacyBuiltinTable{
		{"video_summary", mediaDB, []string{"key", "value"}},
		{"web_ltp_page_cache", webDB, []string{"url", "title", "domain", "summary", "query_hint", "fetched_at"}},
		{"file_mapping", textDB, []string{"key", "value"}},
		{"painter_characters", characterDB, []string{"key", "value"}},
	}

	// knowledgeValidName 库名/表名白名单（防路径穿越与表名拼接注入）
	knowledgeValidName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)
