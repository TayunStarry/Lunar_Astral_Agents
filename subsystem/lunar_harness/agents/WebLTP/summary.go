package WebLTP

// summary.go 页面摘要流水线
//
// 页面滚动过程中持续投递正文快照，后台协程按段落生成摘要；
// 滚动结束后 Finish 等待全部摘要完成（此时页面停留在底部），再合并成单页情报。
// 全程只使用文本，不包含任何截图或视觉识别路径。

import (
	"fmt"
	"strings"
)

// newPageSummarizer 创建摘要流水线并启动后台协程
func newPageSummarizer(query string, hit SearchHit, cfg Config, profile summaryProfile) *pageSummarizer {
	chunkChars := profile.ChunkChars
	if chunkChars <= 0 {
		chunkChars = cfg.TextChunkChars
	}
	maxChunks := profile.MaxChunks
	if maxChunks <= 0 {
		maxChunks = cfg.TextMaxChunks
	}
	if maxChunks <= 0 {
		maxChunks = 1
	}
	s := &pageSummarizer{
		query:      query,
		hit:        hit,
		profile:    profile,
		chunkChars: chunkChars,
		maxChunks:  maxChunks,
		jobs:       make(chan summaryJob, maxChunks),
		done:       make(chan struct{}),
		results:    make([]string, maxChunks),
	}
	go s.run()
	return s
}

// Feed 投递一次正文快照：只取上次之后的新增内容，攒满一段就交给后台摘要
func (s *pageSummarizer) Feed(text string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	runes := []rune(text)
	if len(runes) <= s.consumed {
		return
	}
	s.pending.WriteString(string(runes[s.consumed:]))
	s.consumed = len(runes)

	for s.dispatched < s.maxChunks {
		chunk, ok := takeChunk(&s.pending, s.chunkChars)
		if !ok {
			return
		}
		s.dispatch(chunk)
	}
}

// Finish 结束投递并等待全部摘要完成，返回单页摘要与段数
func (s *pageSummarizer) Finish() (string, int, error) {
	s.mu.Lock()
	if rest := strings.TrimSpace(s.pending.String()); rest != "" && s.dispatched < s.maxChunks {
		s.dispatch(rest)
	}
	s.pending.Reset()
	s.mu.Unlock()

	close(s.jobs)
	<-s.done

	parts := make([]string, 0, s.dispatched)
	for _, part := range s.results[:s.dispatched] {
		if strings.TrimSpace(part) != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		if s.firstErr != nil {
			return "", 0, s.firstErr
		}
		return "", 0, fmt.Errorf("页面无可提取文本")
	}
	if len(parts) == 1 || !s.profile.Merge {
		// 小模型链路：多段直接拼接，省掉一次合并调用（最终报告会统一收口）
		return strings.Join(parts, "\n"), len(parts), nil
	}
	merged, err := mergeChunkSummaries(s.query, s.hit, parts, s.profile)
	if err != nil {
		// 合并失败就按段拼接，保证情报不丢
		return strings.Join(parts, "\n"), len(parts), nil
	}
	return merged, len(parts), nil
}

// run 后台逐段摘要（单协程顺序调用模型，避免并发抢占同一个本地模型）
func (s *pageSummarizer) run() {
	defer close(s.done)
	for job := range s.jobs {
		part, err := summarizeChunk(s.query, s.hit, job.chunk, job.index+1, s.profile)
		if err != nil {
			if s.firstErr == nil {
				s.firstErr = err
			}
			continue
		}
		if strings.TrimSpace(part) != "" {
			s.results[job.index] = part
		}
	}
}

// dispatch 投递一段摘要任务（调用方须持有 s.mu；队列容量等于段数上限，投递不阻塞）
func (s *pageSummarizer) dispatch(chunk string) {
	s.jobs <- summaryJob{index: s.dispatched, chunk: chunk}
	s.dispatched++
}

// takeChunk 从缓冲区切出一段正文：达到段长后优先在末尾换行处断开，避免切断句子
func takeChunk(buf *strings.Builder, limit int) (string, bool) {
	if limit <= 0 {
		limit = 5000
	}
	runes := []rune(buf.String())
	if len(runes) < limit {
		return "", false
	}
	cut := limit
	for i := limit - 1; i >= limit*9/10 && i > 0; i-- {
		if runes[i] == '\n' {
			cut = i + 1
			break
		}
	}
	chunk := strings.TrimSpace(string(runes[:cut]))
	rest := strings.TrimLeft(string(runes[cut:]), "\n")
	buf.Reset()
	buf.WriteString(rest)
	return chunk, chunk != ""
}

// summarizeChunk 单段文本摘要
func summarizeChunk(query string, hit SearchHit, chunk string, index int, profile summaryProfile) (string, error) {
	system, userTemplate := loadPromptPair("webSearchChunkSummary.md")
	user := renderPrompt(userTemplate,
		"{query}", query,
		"{title}", firstNonEmpty(hit.Title, "（无标题）"),
		"{url}", hit.URL,
		"{index}", fmt.Sprintf("%d", index),
		"{chunk}", chunk)
	return chatWithOptions([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, profileChatOptions(profile))
}

// mergeChunkSummaries 把同一页面的多段要点合并为单页摘要
func mergeChunkSummaries(query string, hit SearchHit, parts []string, profile summaryProfile) (string, error) {
	var b strings.Builder
	for i, part := range parts {
		fmt.Fprintf(&b, "【第%d段要点】\n%s\n\n", i+1, part)
	}
	system, userTemplate := loadPromptPair("webSearchMergeSummary.md")
	user := renderPrompt(userTemplate,
		"{query}", query,
		"{title}", firstNonEmpty(hit.Title, "（无标题）"),
		"{url}", hit.URL,
		"{parts}", b.String())
	return chatWithOptions([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, profileChatOptions(profile))
}
