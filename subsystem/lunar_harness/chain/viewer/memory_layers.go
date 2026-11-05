package viewer

// memory_layers.go 观影记忆的分层维持逻辑。
//
// 观影记忆只服务两件事：维护评论倾向、帮助选片。因此不记录每轮发生了什么，而是维持一份
// 当前有效的状态：
//   - 近期观感：每个视频一条，最多 viewerstore.ObservationMax 条，先进先出（淘汰由存储层完成）；
//   - 选片口味：单行，每轮由近期观感的主题聚合重算（观感轮转时偏好会自然变化）；
//   - 评论倾向：单行，每轮用本轮体验覆盖（始终是当前倾向，而不是历史叠加）。

import (
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// updateCommentStance 用本轮体验覆盖「评论倾向」单行。
func updateCommentStance(roundText string) {
	body := viewerstore.Conclusion(cleanMemoryText(roundText))
	if body == "" {
		return
	}
	if err := viewerstore.SaveStance(body); err != nil {
		logger.Warn("BilibiliViewer", "更新评论倾向失败: %v", err)
	}
}

// refreshSelectionProfile 用当前近期观感的主题重算「选片口味」单行。
//
// 这是「渐进修正」的实现：偏好来自最近 viewerstore.ObservationMax 个视频；
// 观感轮转后旧主题自然淡出，不需要额外的模型调用，也不会永远停在旧偏好上。
func refreshSelectionProfile() {
	observations, err := viewerstore.LoadObservations()
	if err != nil {
		logger.Warn("BilibiliViewer", "读取近期观感失败: %v", err)
		return
	}
	if err := viewerstore.SaveProfile(buildSelectionProfile(observations)); err != nil {
		logger.Warn("BilibiliViewer", "更新选片口味失败: %v", err)
	}
}

// buildSelectionProfile 统计近期观感的主题词，按出现次数排序取前若干个。
func buildSelectionProfile(observations []viewerstore.Observation) string {
	counts := make(map[string]int, 16)
	order := make([]string, 0, 16)
	for _, obs := range observations {
		for _, topic := range splitObservationTopics(obs.Topics) {
			if counts[topic] == 0 {
				order = append(order, topic)
			}
			counts[topic]++
		}
	}
	if len(order) == 0 {
		return "还在慢慢摸索自己喜欢的口味。"
	}
	sort.SliceStable(order, func(i, j int) bool {
		if counts[order[i]] != counts[order[j]] {
			return counts[order[i]] > counts[order[j]]
		}
		return order[i] < order[j]
	})
	if len(order) > viewerstore.ProfileTopicMax {
		order = order[:viewerstore.ProfileTopicMax]
	}
	return "最近常看 " + strings.Join(order, "、") + "。"
}

// splitObservationTopics 拆出观感里已压成「A、B」的主题词。
func splitObservationTopics(topics string) []string {
	if strings.TrimSpace(topics) == "" {
		return nil
	}
	parts := strings.Split(topics, "、")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || viewerstore.LooksMechanical(part) {
			continue
		}
		out = append(out, part)
	}
	return out
}
