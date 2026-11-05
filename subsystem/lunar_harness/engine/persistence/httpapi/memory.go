package httpapi

// memory.go 记忆库端点统一分发

import (
	"fmt"
	"net/http"
	"strings"
)

// =============================================================================
// 记忆库端点 — 集合是内容自动归入的分组，查询只按类型（text/image/audio）
//
// 存储布局：<MemoryDBDir>/c-<sha256(簇 ID)[:8]>/{metadata.json, documents_*.json, embeddings_*.json}
// 分组由 clusters.json 的簇 ID 决定，一个簇一个目录；查询/写入不指定分组。
//
// URL 布局：
//
//	POST   /memory/init                 实例初始化（执行布局迁移并加载全部簇目录）
//	GET    /memory/stats                全局统计（按类型聚合）
//	GET    /memory/collections          列出全部簇目录
//	GET    /memory/messages             全局检索（query + type + top_k）
//	POST   /memory/messages             添加记忆（type=text/image/audio，内容自动归组）
//	DELETE /memory/messages             按 id 删除记忆
//	GET    /memory/entries              逐条列举（id=按 ID；type/offset/limit；limit=0 全部）
//	POST   /memory/fuse                 权重融合（把簇内超高相似度的文本条目合并为一条并重算向量）
//	DELETE /memory/{clusterID}          删除簇
//	GET    /memory/{clusterID}/stats    簇统计
//	GET    /memory/{clusterID}/documents 簇内文档分页（管理用途）
//	POST   /memory/{clusterID}/rebuild  重建簇内容向量
//	POST   /memory/{clusterID}/reload   外部改写分片后刷新内存缓存（不重算向量）
//	POST   /memory/{clusterID}/clear    清空簇
// =============================================================================

// MemoryHandler 记忆库统一分发器
func MemoryHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/memory/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusNotFound, "记忆库请求[ERROR] -> 路径不能为空")
		return
	}

	parts := strings.Split(path, "/")

	// 单段路径：全局端点，或按分组名删除集合
	if len(parts) == 1 {
		switch parts[0] {
		case "init":
			handleMemoryInit(w, r)
			return
		case "stats":
			handleMemoryGlobalStats(w, r)
			return
		case "collections":
			handleMemoryListCollections(w, r)
			return
		case "messages":
			handleMemoryMessages(w, r)
			return
		case "entries":
			handleMemoryEntries(w, r)
			return
		case "fuse":
			handleMemoryFuse(w, r)
			return
		}
		if r.Method == http.MethodDelete {
			handleMemoryDeleteCollection(w, r, parts[0])
			return
		}
		writeError(w, http.StatusNotFound, fmt.Sprintf("记忆库请求[ERROR] -> 未知的记忆库端点: %s", parts[0]))
		return
	}

	// 两段路径：/memory/{name}/{action}
	collectionName := parts[0]
	action := parts[1]
	switch action {
	case "stats":
		handleMemoryCollectionStats(w, r, collectionName)
	case "documents":
		handleMemoryDocuments(w, r, collectionName)
	case "rebuild":
		handleMemoryRebuild(w, r, collectionName)
	case "reload":
		handleMemoryReloadCollection(w, r, collectionName)
	case "clear":
		handleMemoryClearCollection(w, r, collectionName)
	default:
		writeError(w, http.StatusNotFound, fmt.Sprintf("记忆库请求[ERROR] -> 未知的集合操作: %s", action))
	}
}

// handleMemoryInit POST /memory/init — 实例初始化（全部集合 + 默认分组 + 集合名向量）
