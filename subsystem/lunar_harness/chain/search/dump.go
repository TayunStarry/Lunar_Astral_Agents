package search

// dump.go 搜索链的导出：最近一次检索的意图、引擎、页数与报告全文。

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/executors/SearchCrawler"
	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

type searchRecord struct {
	Query     string    `json:"query"`
	Engine    string    `json:"engine"`
	Pages     int       `json:"pages"`
	Report    string    `json:"report"`
	StartedAt time.Time `json:"started_at"`
	At        time.Time `json:"at"`
}

var (
	lastSearchMu sync.Mutex
	lastSearch   *searchRecord
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "web_search",
		Label:       "网络搜索",
		Group:       "Go 智能体",
		Description: "最近一次网络检索的检索词、引擎、页数与结构化报告",
		Export: func(includeImages bool) (any, error) {
			lastSearchMu.Lock()
			record := lastSearch
			lastSearchMu.Unlock()
			if record == nil {
				return map[string]any{
					"kind":          "web-search-context",
					"generated_at":  time.Now().Format(time.RFC3339),
					"message_count": 0,
					"note":          "本次进程内还没有执行过网络搜索",
					"config":        SearchCrawler.LoadConfig(),
				}, nil
			}
			return map[string]any{
				"kind":          "web-search-context",
				"generated_at":  time.Now().Format(time.RFC3339),
				"message_count": 1,
				"last_search":   record,
			}, nil
		},
	})
}

func recordLastSearch(query, engine, report string, pages int, started time.Time) {
	lastSearchMu.Lock()
	defer lastSearchMu.Unlock()
	lastSearch = &searchRecord{
		Query:     query,
		Engine:    engine,
		Pages:     pages,
		Report:    report,
		StartedAt: started,
		At:        time.Now(),
	}
}
