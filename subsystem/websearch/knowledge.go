package websearch

import (
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SearchKnowledge 搜索知识库，基于SQLite持久化搜索结果和URL内容
type SearchKnowledge struct {
	db        *sql.DB
	debugLog  func(format string, args ...interface{})
	fts5Ready bool // FTS5是否可用
}

// NewSearchKnowledge 创建搜索知识库，自动建表
// skipSearchTable: 智能学习模式下跳过 search_results 表（向量知识库替代），仅保留 url_content
func NewSearchKnowledge(dbPath string, debugLog func(format string, args ...interface{}), skipSearchTable ...bool) (*SearchKnowledge, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建知识库目录失败: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("打开知识库失败: %w", err)
	}

	skip := len(skipSearchTable) > 0 && skipSearchTable[0]
	sk := &SearchKnowledge{db: db, debugLog: debugLog}
	if err := sk.initTables(skip); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化知识库表失败: %w", err)
	}

	if debugLog != nil {
		debugLog("[搜索知识库] 已初始化 db=%s", dbPath)
	}
	return sk, nil
}

// initTables 创建数据表，skipSearch 时跳过 search_results 和 FTS5（向量模式仅需 url_content）
func (sk *SearchKnowledge) initTables(skipSearch bool) error {
	// URL 内容缓存表（所有模式都需要）
	_, err := sk.db.Exec(`
		CREATE TABLE IF NOT EXISTS url_content (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			url_hash    TEXT NOT NULL,
			url         TEXT NOT NULL,
			title       TEXT NOT NULL,
			content     TEXT NOT NULL,
			source_type TEXT NOT NULL DEFAULT 'unknown',
			fetched_at  DATETIME NOT NULL,
			updated_at  DATETIME NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_url_hash ON url_content(url_hash);
	`)
	if err != nil {
		return err
	}

	// 智能学习模式：跳过 search_results 表（向量知识库替代）
	if skipSearch {
		if sk.debugLog != nil {
			sk.debugLog("[搜索知识库] 向量模式，跳过 search_results 表，仅保留 url_content")
		}
		return nil
	}

	// 创建 search_results 表（旧逻辑）
	_, err = sk.db.Exec(`
		CREATE TABLE IF NOT EXISTS search_results (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			query_hash  TEXT NOT NULL,
			query_text  TEXT NOT NULL,
			results     TEXT NOT NULL,
			searched_at DATETIME NOT NULL,
			updated_at  DATETIME NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_query_hash ON search_results(query_hash);
	`)
	if err != nil {
		return err
	}

	// 迁移：为旧数据库添加 result_text 列（如果不存在）
	_, err = sk.db.Exec(`ALTER TABLE search_results ADD COLUMN result_text TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		// 如果列已经存在，SQLite会返回错误，忽略即可
		if !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
		if sk.debugLog != nil {
			sk.debugLog("[搜索知识库] result_text列已存在，跳过迁移")
		}
	}

	// 创建FTS5虚拟表用于模糊查询（同时索引query_text和result_text，提高匹配精度）
	_, err = sk.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS search_results_fts USING fts5(
			query_text,
			result_text,
			content='search_results',
			content_rowid='id'
		);
	`)
	if err != nil {
		// FTS5可能不支持（如旧版SQLite），降级处理
		if sk.debugLog != nil {
			sk.debugLog("[搜索知识库] FTS5创建失败，将使用关键词匹配降级方案: %v", err)
		}
	} else {
		sk.fts5Ready = true

		// 创建触发器保持FTS索引同步（同步query_text和result_text）
		_, err = sk.db.Exec(`
			CREATE TRIGGER IF NOT EXISTS search_results_fts_insert AFTER INSERT ON search_results
			BEGIN
				INSERT INTO search_results_fts(rowid, query_text, result_text) VALUES (new.id, new.query_text, new.result_text);
			END;
			CREATE TRIGGER IF NOT EXISTS search_results_fts_update AFTER UPDATE ON search_results
			BEGIN
				INSERT OR REPLACE INTO search_results_fts(rowid, query_text, result_text) VALUES (new.id, new.query_text, new.result_text);
			END;
			CREATE TRIGGER IF NOT EXISTS search_results_fts_delete AFTER DELETE ON search_results
			BEGIN
				INSERT INTO search_results_fts(search_results_fts, rowid, query_text, result_text) VALUES ('delete', old.id, old.query_text, old.result_text);
			END;
		`)
		if err != nil {
			if sk.debugLog != nil {
				sk.debugLog("[搜索知识库] FTS触发器创建失败: %v", err)
			}
		}
	}

	return nil
}

// Close 关闭知识库
func (sk *SearchKnowledge) Close() error {
	if sk.db != nil {
		return sk.db.Close()
	}
	return nil
}

// hashQuery 标准化查询词的哈希
func hashQuery(query string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(query)))
}

// hashURL URL 的哈希
func hashURL(url string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(url)))
}

// cachedResult 存储的搜索结果
type cachedResult struct {
	ResultText string         `json:"result_text"`
	Results    []SearchResult `json:"results"`
	SearchedAt time.Time      `json:"searched_at"`
}

// LookupQuery 查询知识库中是否有此查询的结果
// 先精确匹配，未命中则尝试FTS5模糊匹配，最后降级为关键词匹配
func (sk *SearchKnowledge) LookupQuery(query string) (*cachedResult, error) {
	return sk.LookupQueryWithTimeRange(query, 0)
}

// LookupQueryWithTimeRange 查询知识库（精确→FTS5模糊→关键词匹配三级降级），maxAge=0不过滤时间
func (sk *SearchKnowledge) LookupQueryWithTimeRange(query string, maxAge time.Duration) (*cachedResult, error) {
	if sk.debugLog != nil {
		sk.debugLog("[搜索知识库] 开始查询 query=%q maxAge=%v", query, maxAge)
	}

	// 构建时间过滤条件
	var timeCondition string
	var timeArgs []interface{}
	if maxAge > 0 {
		timeCondition = " AND searched_at >= ?"
		timeArgs = append(timeArgs, time.Now().Add(-maxAge))
	}

	// 先精确匹配
	queryHash := hashQuery(query)
	args := append([]interface{}{queryHash}, timeArgs...)
	row := sk.db.QueryRow(
		"SELECT result_text, results, searched_at FROM search_results WHERE query_hash = ?"+timeCondition,
		args...,
	)

	var resultText, resultsJSON string
	var searchedAt time.Time
	if err := row.Scan(&resultText, &resultsJSON, &searchedAt); err != nil {
		if err != sql.ErrNoRows {
			return nil, err
		}
		// 精确匹配未命中，根据FTS5可用性选择查询方式
		var cr *cachedResult
		if sk.fts5Ready {
			// FTS5可用，尝试模糊匹配
			var ftsErr error
			cr, ftsErr = sk.ftsLookupWithTimeRange(query, maxAge)
			if ftsErr != nil {
				if sk.debugLog != nil {
					sk.debugLog("[搜索知识库] FTS5查询失败，尝试关键词匹配: %v", ftsErr)
				}
				cr, _ = sk.keywordLookupWithTimeRange(query, maxAge)
			}
		} else {
			// FTS5不可用，直接使用关键词匹配
			cr, _ = sk.keywordLookupWithTimeRange(query, maxAge)
		}
		if cr != nil {
			return cr, nil
		}
		if sk.debugLog != nil {
			sk.debugLog("[搜索知识库] 未命中 query=%q（精确匹配无结果）", query)
		}
		return nil, nil
	}

	var cr cachedResult
	if err := json.Unmarshal([]byte(resultsJSON), &cr.Results); err != nil {
		return nil, fmt.Errorf("解析缓存结果失败: %w", err)
	}
	cr.ResultText = resultText
	cr.SearchedAt = searchedAt

	if sk.debugLog != nil {
		sk.debugLog("[搜索知识库] 精确命中 query=%q 时间=%s 结果数=%d",
			query, searchedAt.Format("01-02 15:04"), len(cr.Results))
	}
	return &cr, nil
}

// ftsLookup 使用FTS5进行模糊查询（同时搜索query_text和result_text）
func (sk *SearchKnowledge) ftsLookup(query string) (*cachedResult, error) {
	return sk.ftsLookupWithTimeRange(query, 0)
}

// ftsLookupWithTimeRange 使用FTS5进行模糊查询（带时间范围过滤）
func (sk *SearchKnowledge) ftsLookupWithTimeRange(query string, maxAge time.Duration) (*cachedResult, error) {
	// 将查询词转为FTS5查询格式（空格分隔的OR查询）
	ftsQuery := strings.ReplaceAll(strings.TrimSpace(query), " ", " OR ")

	// 构建时间过滤条件
	var timeCondition string
	var args []interface{} = []interface{}{ftsQuery}
	if maxAge > 0 {
		timeCondition = " AND r.searched_at >= ?"
		args = append(args, time.Now().Add(-maxAge))
	}

	rows, err := sk.db.Query(`
			SELECT r.result_text, r.results, r.searched_at, r.query_text
			FROM search_results r
			JOIN search_results_fts ON r.id = search_results_fts.rowid
			WHERE search_results_fts MATCH ?`+timeCondition+`
			ORDER BY rank
			LIMIT 1
		`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if rows.Next() {
		var resultText, resultsJSON, matchedQuery string
		var searchedAt time.Time
		if err := rows.Scan(&resultText, &resultsJSON, &searchedAt, &matchedQuery); err != nil {
			return nil, err
		}

		var cr cachedResult
		if err := json.Unmarshal([]byte(resultsJSON), &cr.Results); err != nil {
			return nil, err
		}
		cr.ResultText = resultText
		cr.SearchedAt = searchedAt

		if sk.debugLog != nil {
			sk.debugLog("[搜索知识库] FTS5模糊匹配命中 query=%q → matched=%q 时间=%s 结果数=%d",
				query, matchedQuery, searchedAt.Format("01-02 15:04"), len(cr.Results))
		}
		return &cr, nil
	}
	return nil, nil
}

// keywordLookup 关键词模糊匹配（FTS5降级方案），重叠≥2个关键词即命中
func (sk *SearchKnowledge) keywordLookup(query string) (*cachedResult, error) {
	return sk.keywordLookupWithTimeRange(query, 0)
}

// keywordLookupWithTimeRange 关键词模糊匹配（带时间范围过滤）
func (sk *SearchKnowledge) keywordLookupWithTimeRange(query string, maxAge time.Duration) (*cachedResult, error) {
	queryKW := extractKnowledgeKeywords(query)
	if len(queryKW) < 2 {
		return nil, nil
	}

	// 构建时间过滤条件
	var timeCondition string
	var args []interface{}
	if maxAge > 0 {
		timeCondition = " WHERE searched_at >= ?"
		args = append(args, time.Now().Add(-maxAge))
	}

	rows, err := sk.db.Query("SELECT query_text, result_text, results, searched_at FROM search_results"+timeCondition, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bestMatch *cachedResult
	var bestMatchQuery string
	bestScore := 0
	for rows.Next() {
		var storedQuery, resultText, resultsJSON string
		var searchedAt time.Time
		if err := rows.Scan(&storedQuery, &resultText, &resultsJSON, &searchedAt); err != nil {
			continue
		}
		storedKW := extractKnowledgeKeywords(storedQuery)
		score := countKeywordOverlap(queryKW, storedKW)
		if score >= 2 && score > bestScore {
			var cr cachedResult
			if err := json.Unmarshal([]byte(resultsJSON), &cr.Results); err != nil {
				continue
			}
			cr.ResultText = resultText
			cr.SearchedAt = searchedAt
			bestMatch = &cr
			bestMatchQuery = storedQuery
			bestScore = score
		}
	}
	if bestMatch != nil && sk.debugLog != nil {
		sk.debugLog("[搜索知识库] 关键词模糊匹配命中 query=%q → matched=%q 时间=%s 结果数=%d",
			query, bestMatchQuery, bestMatch.SearchedAt.Format("01-02 15:04"), len(bestMatch.Results))
	}
	return bestMatch, nil
}

// extractKnowledgeKeywords 从查询中提取关键词（2字以上的中文词）
func extractKnowledgeKeywords(query string) []string {
	words := make([]string, 0)
	runes := []rune(query)

	parts := strings.FieldsFunc(string(runes), func(r rune) bool {
		return r == ' ' || r == '，' || r == '。' || r == '！' || r == '？' || r == '、' || r == '的' || r == '是' || r == '了' || r == '吗' || r == '呢' || r == '吧' || r == '啊'
	})
	for _, part := range parts {
		partRunes := []rune(part)
		if len(partRunes) >= 2 {
			words = append(words, strings.ToLower(part))
		}
		for i := 0; i < len(partRunes)-1; i++ {
			bigram := string(partRunes[i : i+2])
			words = append(words, strings.ToLower(bigram))
		}
	}

	seen := make(map[string]bool)
	unique := make([]string, 0, len(words))
	for _, w := range words {
		if !seen[w] {
			seen[w] = true
			unique = append(unique, w)
		}
	}
	return unique
}

// countKeywordOverlap 计算两个关键词列表的重叠数量
func countKeywordOverlap(a, b []string) int {
	set := make(map[string]bool, len(b))
	for _, w := range b {
		set[w] = true
	}
	count := 0
	for _, w := range a {
		if set[w] {
			count++
		}
	}
	return count
}

// StoreQuery 存储或更新查询结果（存储最终LLM总结结果+原始SearchResults）
func (sk *SearchKnowledge) StoreQuery(query string, resultText string, results []SearchResult) error {
	queryHash := hashQuery(query)
	now := time.Now()

	resultsJSON, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("序列化结果失败: %w", err)
	}

	_, err = sk.db.Exec(`
		INSERT INTO search_results (query_hash, query_text, result_text, results, searched_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(query_hash) DO UPDATE SET
			result_text = excluded.result_text,
			results = excluded.results,
			searched_at = excluded.searched_at,
			updated_at = excluded.updated_at
	`, queryHash, query, resultText, string(resultsJSON), now, now)

	if err != nil && sk.debugLog != nil {
		sk.debugLog("[搜索知识库] 存储失败 query=%q err=%v", query, err)
	} else if sk.debugLog != nil {
		sk.debugLog("[搜索知识库] 存储成功 query=%q 结果数=%d", query, len(results))
	}
	return err
}

// cachedURLContent 存储的 URL 内容
type cachedURLContent struct {
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	SourceType string    `json:"source_type"`
	FetchedAt  time.Time `json:"fetched_at"`
}

// LookupURL 查询知识库中是否有此 URL 的内容
// maxAge: 缓存最大有效期，0 表示不检查时效性
func (sk *SearchKnowledge) LookupURL(url string, maxAge ...time.Duration) (*cachedURLContent, error) {
	urlHash := hashURL(url)
	row := sk.db.QueryRow(
		"SELECT title, content, source_type, fetched_at FROM url_content WHERE url_hash = ?",
		urlHash,
	)

	var c cachedURLContent
	if err := row.Scan(&c.Title, &c.Content, &c.SourceType, &c.FetchedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// 时效性检查
	if len(maxAge) > 0 && maxAge[0] > 0 {
		if time.Since(c.FetchedAt) > maxAge[0] {
			if sk.debugLog != nil {
				sk.debugLog("[搜索知识库] URL缓存过期 url=%s 年龄=%s 限制=%s", url, time.Since(c.FetchedAt).Round(time.Second), maxAge[0])
			}
			return nil, nil
		}
	}

	if sk.debugLog != nil {
		sk.debugLog("[搜索知识库] URL命中 url=%s 时间=%s 年龄=%s", url, c.FetchedAt.Format("01-02 15:04"), time.Since(c.FetchedAt).Round(time.Minute))
	}
	return &c, nil
}

// StoreURL 存储或更新URL内容缓存
func (sk *SearchKnowledge) StoreURL(url, title, content, sourceType string) error {
	if sourceType == "" {
		sourceType = "unknown"
	}
	urlHash := hashURL(url)
	now := time.Now()

	_, err := sk.db.Exec(`
		INSERT INTO url_content (url_hash, url, title, content, source_type, fetched_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(url_hash) DO UPDATE SET
			title = excluded.title,
			content = excluded.content,
			source_type = excluded.source_type,
			fetched_at = excluded.fetched_at,
			updated_at = excluded.updated_at
	`, urlHash, url, title, content, sourceType, now, now)

	if err != nil && sk.debugLog != nil {
		sk.debugLog("[搜索知识库] URL存储失败 url=%s err=%v", url, err)
	} else if sk.debugLog != nil {
		sk.debugLog("[搜索知识库] URL存储成功 url=%s", url)
	}
	return err
}
