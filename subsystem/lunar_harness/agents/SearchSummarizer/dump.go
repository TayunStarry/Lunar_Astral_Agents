package SearchSummarizer

// dump.go 搜索者-摘要的导出：最近一次摘要输入规模与输出全文。

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

type lastDigestRecord struct {
	Query  string    `json:"query"`
	Chars  int       `json:"input_chars"`
	Digest string    `json:"digest"`
	At     time.Time `json:"at"`
}

var (
	lastDigestMu   sync.Mutex
	lastDigestItem *lastDigestRecord
)

// exportContext 搜索者-摘要的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	_ = includeImages
	lastDigestMu.Lock()
	record := lastDigestItem
	lastDigestMu.Unlock()
	if record == nil {
		return map[string]any{
			"kind":          "search-summarizer-context",
			"generated_at":  time.Now().Format(time.RFC3339),
			"message_count": 0,
			"note":          "本次进程内还没有执行过搜索摘要",
		}, nil
	}
	return map[string]any{
		"kind":          "search-summarizer-context",
		"generated_at":  time.Now().Format(time.RFC3339),
		"message_count": 1,
		"last_digest":   record,
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          AgentID,
		Label:       modelRole,
		Group:       "Go 智能体",
		Description: modelRole + "最近一次的检索词、输入规模与摘要全文",
		Export:      exportContext,
	})
}

func recordLastDigest(query, digest string) {
	lastDigestMu.Lock()
	defer lastDigestMu.Unlock()
	lastDigestItem = &lastDigestRecord{
		Query:  query,
		Chars:  len([]rune(digest)),
		Digest: digest,
		At:     time.Now(),
	}
}
