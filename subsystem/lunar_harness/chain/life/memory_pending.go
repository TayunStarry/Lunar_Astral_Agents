package life

// memory_pending.go 待合并（Pending）队列：每轮整理唯一的输入。
//
// 写入侧命中高度相似条目时不会丢弃新措辞，而是写入并标记 Pending + PendingPartner。
// 本文件负责把这些标记连成待合并组（pending → partner 连边 + 聚类并组），
// 交给同一套整理流程（模型合并 → 双向校验 → 归档 → ReplaceTexts 先写后删）。
//
// 整理不做任何基于条数/时间的判定：没有待合并组就整轮不动作（也不消耗额度），
// 有待合并组就逐组处理；某组失败一律保持原状等待下轮重试，不做强制淘汰兜底。

import (
	"context"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/memory"
)

// pendingCandidateSet 收集待处理条目与其伙伴条目。
//
// 伙伴条目本身可能没有 Pending 标记（它是先写入的旧条目），但必须一起进入整理，
// 否则待合并组不完整；伙伴已不存在时由 clearOrphanPending 回收待处理标记。
func pendingCandidateSet(items []memoryItem) (pending, partners []memoryItem) {
	byID := make(map[string]memoryItem, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	seenPartner := make(map[string]struct{})
	for _, item := range items {
		if !item.Pending {
			continue
		}
		pending = append(pending, item)
		partnerID := strings.TrimSpace(item.PendingPartner)
		if partnerID == "" {
			continue
		}
		if _, ok := seenPartner[partnerID]; ok {
			continue
		}
		seenPartner[partnerID] = struct{}{}
		if partner, ok := byID[partnerID]; ok {
			partners = append(partners, partner)
		}
	}
	return pending, partners
}

// clearOrphanPending 处理伙伴已不存在的待处理条目：清除其待处理标记，让条目回到可见状态。
//
// 伙伴被其它批次合并或手工删除后，单条 pending 无法再组成待合并组；若不清除标记，
// 它会永远对检索隐藏。清除失败则保留下轮重试，绝不先删数据。
func clearOrphanPending(ctx context.Context, pending, partners []memoryItem) []memoryItem {
	found := make(map[string]struct{}, len(partners))
	for _, item := range partners {
		found[item.ID] = struct{}{}
	}
	kept := make([]memoryItem, 0, len(pending))
	for _, item := range pending {
		partnerID := strings.TrimSpace(item.PendingPartner)
		if _, ok := found[partnerID]; partnerID != "" && ok {
			kept = append(kept, item)
			continue
		}
		if err := pendingClearHook(ctx, item.ID); err != nil {
			logger.Warn(Module, "[记忆整理] 清除孤儿待处理标记失败（保留待下轮）: ID=%s 原因=%v", item.ID, err)
			kept = append(kept, item)
		}
	}
	return kept
}

// pendingGroupItems 合并待处理条目与伙伴条目并按 ID 去重。
// 一个待处理条目也可能同时是其它条目的伙伴，去重后仍只进入一个批次。
func pendingGroupItems(pending, partners []memoryItem) []memoryItem {
	seen := make(map[string]struct{}, len(pending)+len(partners))
	out := make([]memoryItem, 0, len(pending)+len(partners))
	for _, list := range [][]memoryItem{pending, partners} {
		for _, item := range list {
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

// planPendingBatches 把当前待合并条目规划成本轮的整理批次。
// 没有待合并组时返回 nil 与原因，调用方据此直接结束本轮（不产生任何动作）。
func planPendingBatches(groupItems []memoryItem, clusters []memory.TextCluster) ([]memoryBatch, string) {
	if len(groupItems) == 0 {
		return nil, "没有待合并的记忆"
	}
	batches := buildPendingBatches(groupItems, clusters)
	if len(batches) == 0 {
		return nil, "待合并组没有可合并的相似内容，本次全部原样保留"
	}
	return batches, ""
}

// buildPendingBatches 把待处理组映射成批次。
//
// 分组来自两处：pending → partner 的直接连边（保证即使聚类阈值未命中也不丢组），
// 以及聚类结果（把同簇的其它待合并条目并到一起）。只输出含 Pending 条目且 ≥2 条的组；
// 组过大时按固定大小切块，保证单批模型输入有界。批次按组内最旧条目时间升序排列，
// 保证多组按「旧记忆优先」的顺序逐组处理。
func buildPendingBatches(items []memoryItem, clusters []memory.TextCluster) []memoryBatch {
	byID := make(map[string]memoryItem, len(items))
	ids := make([]string, 0, len(items))
	for _, item := range items {
		byID[item.ID] = item
		ids = append(ids, item.ID)
	}
	if len(ids) == 0 {
		return nil
	}

	uf := newIDUnionFind(ids)
	for _, item := range items {
		if !item.Pending {
			continue
		}
		partnerID := strings.TrimSpace(item.PendingPartner)
		if _, ok := byID[partnerID]; ok {
			uf.union(item.ID, partnerID)
		}
	}
	for _, cluster := range clusters {
		first := ""
		for _, id := range cluster.IDs {
			if _, ok := byID[id]; !ok {
				continue
			}
			if first == "" {
				first = id
				continue
			}
			uf.union(first, id)
		}
	}

	batches := make([]memoryBatch, 0)
	for _, groupIDs := range uf.components() {
		groupItems := make([]memoryItem, 0, len(groupIDs))
		hasPending := false
		for _, id := range groupIDs {
			item, ok := byID[id]
			if !ok {
				continue
			}
			groupItems = append(groupItems, item)
			if item.Pending {
				hasPending = true
			}
		}
		if len(groupItems) < 2 || !hasPending {
			continue
		}
		sort.SliceStable(groupItems, func(i, j int) bool {
			if groupItems[i].Timestamp != groupItems[j].Timestamp {
				return groupItems[i].Timestamp < groupItems[j].Timestamp
			}
			return groupItems[i].ID < groupItems[j].ID
		})
		// 组过大时按固定大小切块，保证单批模型输入有界；切块后仍按最旧优先排序。
		for _, chunk := range chunkPendingGroup(groupItems) {
			if len(chunk) < 2 || !groupHasPending(chunk) {
				continue
			}
			batches = append(batches, memoryBatch{Items: chunk})
		}
	}
	sort.SliceStable(batches, func(i, j int) bool {
		if oldest := batchOldestTimestamp(batches[i]); oldest != batchOldestTimestamp(batches[j]) {
			return oldest < batchOldestTimestamp(batches[j])
		}
		return batches[i].Items[0].ID < batches[j].Items[0].ID
	})
	return batches
}

// chunkPendingGroup 把过大的待合并组切成若干块，保证每块至少含一个 Pending 条目。
//
// 组按入库时间升序切块时可能出现「纯伙伴块」（伙伴都比待处理条目旧）；
// 若直接跳过这种块，与之对应的待处理条目就失去了合并机会，
// 因此把它并入相邻块，宁可单块略超上限也不丢组。
func chunkPendingGroup(items []memoryItem) [][]memoryItem {
	if len(items) <= pendingGroupMaxItems {
		return [][]memoryItem{items}
	}
	chunks := make([][]memoryItem, 0, len(items)/pendingGroupMaxItems+1)
	for start := 0; start < len(items); start += pendingGroupMaxItems {
		end := start + pendingGroupMaxItems
		if end > len(items) {
			end = len(items)
		}
		chunk := append([]memoryItem(nil), items[start:end]...)
		if !groupHasPending(chunk) && len(chunks) > 0 {
			chunks[len(chunks)-1] = append(chunks[len(chunks)-1], chunk...)
			continue
		}
		chunks = append(chunks, chunk)
	}
	// 首块纯伙伴时向后并入，避免整组开头被跳过。
	if len(chunks) > 1 && !groupHasPending(chunks[0]) {
		chunks[1] = append(append([]memoryItem(nil), chunks[0]...), chunks[1]...)
		chunks = chunks[1:]
	}
	return chunks
}

func groupHasPending(items []memoryItem) bool {
	for _, item := range items {
		if item.Pending {
			return true
		}
	}
	return false
}

// batchOldestTimestamp 批次内条目已按入库时间升序，首条即最旧。
func batchOldestTimestamp(batch memoryBatch) int64 {
	if len(batch.Items) == 0 {
		return 0
	}
	return batch.Items[0].Timestamp
}

// idUnionFind 轻量并查集：把 pending → partner 连边与聚类结果合并成完整待处理组。
type idUnionFind struct {
	parent map[string]string
}

func newIDUnionFind(ids []string) *idUnionFind {
	uf := &idUnionFind{parent: make(map[string]string, len(ids))}
	for _, id := range ids {
		uf.parent[id] = id
	}
	return uf
}

func (u *idUnionFind) find(id string) string {
	root := id
	for u.parent[root] != root {
		root = u.parent[root]
	}
	for id != root {
		next := u.parent[id]
		u.parent[id] = root
		id = next
	}
	return root
}

func (u *idUnionFind) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[rb] = ra
	}
}

// components 返回全部连通分量（保持首次出现的顺序，便于批次排序稳定）。
func (u *idUnionFind) components() [][]string {
	grouped := make(map[string][]string)
	order := make([]string, 0)
	for id := range u.parent {
		root := u.find(id)
		if _, ok := grouped[root]; !ok {
			order = append(order, root)
		}
		grouped[root] = append(grouped[root], id)
	}
	out := make([][]string, 0, len(order))
	for _, root := range order {
		out = append(out, grouped[root])
	}
	return out
}
