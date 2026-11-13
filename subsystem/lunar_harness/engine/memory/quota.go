package memory

// quota.go 配额切分、硬去重与时间排序。
//
// 全部为纯函数：不触发模型、不读写存储、不依赖时钟，便于单测与后续
// 查询层直接复用。S1 只提供实现，不接入现有查询入口。

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// QuotaHit 参与配额分配的单条检索命中（内部结构）。
// 渲染层只允许读取 Timestamp 与 Content，其余字段仅用于配额、去重与排序。
type QuotaHit struct {
	ID         string  // 文档 ID
	SpeakerKey string  // 发言者内部键；为空表示不对应具体聊天对象
	Namespace  string  // 内部分区键
	Content    string  // 正文
	Timestamp  int64   // 入库时间（Unix 秒）
	Score      float32 // 融合相关度
}

// QuotaOptions 配额参数；零值或负值使用默认常量。
type QuotaOptions struct {
	TopK            int // 总召回上限，默认 QuotaTopK
	PerUser         int // 每个聊天对象上限，默认 QuotaPerUser
	MaxParticipants int // 参与分配的聊天对象上限，默认 QuotaMaxParticipants
}

// QuotaResult 配额切分结果。
type QuotaResult struct {
	PerUser map[string][]QuotaHit // 发言者键 → 该用户的命中（按分数降序）
	Global  []QuotaHit            // 剩余全局配额（按分数降序）
}

// AllocateQuota 按「每用户配额 + 全局配额」切分候选。
//
// 规则（与设计文档第 6.1、12.1 节一致）：
//   - participants 去重、去空、按传入顺序取前 MaxParticipants 个；
//   - 每个参与者按分数取前 PerUser 条；
//   - 总数不足 TopK 的余额全部进入 Global，不设每用户下限；
//   - 参与者不足时，空出的配额自然并入 Global；
//   - 同分按 Timestamp 倒序、ID 升序，保证结果稳定。
func AllocateQuota(items []QuotaHit, participants []string, opts QuotaOptions) QuotaResult {
	opts = normalizeQuotaOptions(opts)
	ordered := append([]QuotaHit(nil), items...)
	sortHitsByScore(ordered)

	result := QuotaResult{PerUser: make(map[string][]QuotaHit)}
	used := make(map[int]bool, len(ordered))
	for _, speaker := range normalizeParticipants(participants, opts.MaxParticipants) {
		for index, hit := range ordered {
			if len(result.PerUser[speaker]) >= opts.PerUser || len(used) >= opts.TopK {
				break
			}
			if used[index] || hit.SpeakerKey != speaker {
				continue
			}
			used[index] = true
			result.PerUser[speaker] = append(result.PerUser[speaker], hit)
		}
	}
	for index, hit := range ordered {
		if len(used) >= opts.TopK {
			break
		}
		if used[index] {
			continue
		}
		used[index] = true
		result.Global = append(result.Global, hit)
	}
	return result
}

// DeduplicateHits 按「消息 ID」与「正文哈希」硬去重，保留首次出现的条目。
//
// ID 为空时只看正文哈希；正文为空且 ID 为空时视为不可去重，原样保留。
func DeduplicateHits(items []QuotaHit) []QuotaHit {
	seenIDs := make(map[string]struct{}, len(items))
	seenHashes := make(map[string]struct{}, len(items))
	out := make([]QuotaHit, 0, len(items))
	for _, hit := range items {
		if id := strings.TrimSpace(hit.ID); id != "" {
			if _, ok := seenIDs[id]; ok {
				continue
			}
			seenIDs[id] = struct{}{}
		}
		if hash := ContentHash(hit.Content); hash != "" {
			if _, ok := seenHashes[hash]; ok {
				continue
			}
			seenHashes[hash] = struct{}{}
		}
		out = append(out, hit)
	}
	return out
}

// ContentHash 返回正文的稳定哈希（sha256 前 16 位十六进制）。
// 连续空白归一为单个空格后比较；空正文返回空串。
func ContentHash(content string) string {
	normalized := normalizeContent(content)
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])[:16]
}

// SortHitsByTime 按时间升序原地排序；时间为 0 或负数的条目排在最后，
// 同时间按 ID 升序，保证同一输入渲染顺序稳定。
func SortHitsByTime(items []QuotaHit) {
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		leftKnown, rightKnown := left.Timestamp > 0, right.Timestamp > 0
		if leftKnown != rightKnown {
			return leftKnown
		}
		if left.Timestamp != right.Timestamp {
			return left.Timestamp < right.Timestamp
		}
		return left.ID < right.ID
	})
}

// normalizeQuotaOptions 把零值/负值夹回默认常量。
func normalizeQuotaOptions(opts QuotaOptions) QuotaOptions {
	if opts.TopK <= 0 {
		opts.TopK = QuotaTopK
	}
	if opts.PerUser <= 0 {
		opts.PerUser = QuotaPerUser
	}
	if opts.MaxParticipants <= 0 {
		opts.MaxParticipants = QuotaMaxParticipants
	}
	return opts
}

// normalizeParticipants 去重、去空并按上限截断，保持传入顺序。
func normalizeParticipants(participants []string, limit int) []string {
	seen := make(map[string]struct{}, len(participants))
	out := make([]string, 0, len(participants))
	for _, speaker := range participants {
		speaker = strings.TrimSpace(speaker)
		if speaker == "" {
			continue
		}
		if _, ok := seen[speaker]; ok {
			continue
		}
		seen[speaker] = struct{}{}
		out = append(out, speaker)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// sortHitsByScore 按分数降序、时间倒序、ID 升序排列。
func sortHitsByScore(items []QuotaHit) {
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		if left.Timestamp != right.Timestamp {
			return left.Timestamp > right.Timestamp
		}
		return left.ID < right.ID
	})
}

// normalizeContent 把连续空白归一为单个空格并去掉首尾空白。
func normalizeContent(content string) string {
	return strings.Join(strings.Fields(content), " ")
}
