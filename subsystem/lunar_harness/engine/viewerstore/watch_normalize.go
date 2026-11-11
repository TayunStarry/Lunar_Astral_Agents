package viewerstore

// watch_normalize.go 待看视频入队前的清洗与校验（BV 去重、片段不重叠等）。

import (
	"fmt"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/internal/textutil"
)

// normalizeCandidates 清洗入队候选：BV 去重、文本截断、片段按时间排序并校验不重叠。
//
// 批量内同 BV 只保留第一条；与库内已有的同 BV 记录由入队 SQL 的幂等约束处理。
func normalizeCandidates(items []WatchCandidate) ([]WatchCandidate, error) {
	out := make([]WatchCandidate, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		item.BV = strings.TrimSpace(item.BV)
		if item.BV == "" {
			return nil, fmt.Errorf("待看视频缺少 BV 号")
		}
		if !IsBVID(item.BV) {
			return nil, fmt.Errorf("BV 号格式不合法: %s", item.BV)
		}
		if seen[item.BV] {
			continue
		}
		seen[item.BV] = true
		item.Title = strings.TrimSpace(item.Title)
		item.Intro = textutil.ClipRunes(strings.TrimSpace(item.Intro), WatchIntroMaxRunes)
		item.AuthorUID = strings.TrimSpace(item.AuthorUID)
		item.AuthorName = strings.TrimSpace(item.AuthorName)
		item.Comments = normalizeComments(item.Comments)
		item.Topics = NormalizeTopics(item.Topics)
		if item.DurationSec < 0 {
			item.DurationSec = 0
		}
		segments, err := normalizeSegments(item.Segments, item.DurationSec)
		if err != nil {
			return nil, fmt.Errorf("视频 %s 的片段不合法: %w", item.BV, err)
		}
		item.Segments = segments
		out = append(out, item)
	}
	return out, nil
}

// normalizeComments 去空白、去超长、去重后保留前 WatchCommentsMax 条。
func normalizeComments(comments []string) []string {
	out := make([]string, 0, len(comments))
	seen := make(map[string]bool, len(comments))
	for _, comment := range comments {
		comment = strings.TrimSpace(comment)
		if comment == "" || seen[comment] {
			continue
		}
		seen[comment] = true
		out = append(out, comment)
		if len(out) >= WatchCommentsMax {
			break
		}
	}
	return out
}

// NormalizeTopics 清洗主题词：去空白、按大小写不敏感去重、截断超长，保留前 WatchTopicsMax 个。
//
// 入队写入与主题词回流共用这一份规范化规则，保证两处对同一组主题词的判定一致。
func NormalizeTopics(topics []string) []string {
	out := make([]string, 0, len(topics))
	seen := make(map[string]bool, len(topics))
	for _, topic := range topics {
		topic = textutil.ClipRunes(strings.TrimSpace(topic), WatchTopicMaxRunes)
		if topic == "" {
			continue
		}
		key := strings.ToLower(topic)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, topic)
		if len(out) >= WatchTopicsMax {
			break
		}
	}
	return out
}

// normalizeSegments 校验片段数量与区间，按起始时间排序后确认互不重叠。
//
// 序号不在这里分配：落库时按排序后的位置写入 seg_index，调用方无需关心。
func normalizeSegments(input []SegmentInput, duration int) ([]SegmentInput, error) {
	if len(input) > WatchSegmentMax {
		return nil, fmt.Errorf("片段数量 %d 超过上限 %d", len(input), WatchSegmentMax)
	}
	out := make([]SegmentInput, 0, len(input))
	for _, seg := range input {
		seg.Path = strings.TrimSpace(seg.Path)
		switch {
		case seg.StartSec < 0:
			return nil, fmt.Errorf("起始时间不能为负")
		case seg.EndSec <= seg.StartSec:
			return nil, fmt.Errorf("结束时间必须大于起始时间")
		case duration > 0 && seg.EndSec > duration:
			return nil, fmt.Errorf("结束时间超过视频时长 %d", duration)
		}
		if seg.Path == "" {
			seg.Downloaded = false
		}
		out = append(out, seg)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartSec < out[j].StartSec })
	for index := 1; index < len(out); index++ {
		if out[index].StartSec < out[index-1].EndSec {
			return nil, fmt.Errorf("第 %d 段与第 %d 段时间重叠", index, index+1)
		}
	}
	return out, nil
}

// validWatchStatus 判断审核状态取值是否合法。
func validWatchStatus(status WatchStatus) bool {
	switch status {
	case WatchStatusPending, WatchStatusApproved, WatchStatusRejected:
		return true
	default:
		return false
	}
}
