package httpapi

// memory_stats.go 全局/集合统计端点

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/memory"
	"fmt"
	"net/http"
)

func handleMemoryInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	if err := memory.EnsureReady(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 实例初始化失败: %v", err))
		return
	}

	logger.Info("FileManager", "记忆库实例初始化成功（全部集合已加载，默认分组与集合名向量已就绪）")

	writeSuccess(w, map[string]string{
		"message": "记忆库实例初始化成功",
	})
}

// handleMemoryGlobalStats GET /memory/stats — 全局统计（按类型聚合）
func handleMemoryGlobalStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	if !memory.Ready() {
		writeSuccess(w, memoryStatsData{Initialized: false})
		return
	}

	totalDocs := 0
	mismatch := false
	for _, name := range memory.Collections() {
		totalDocs += memory.DocCount(name)
		if memory.HasSyncMismatch(name) {
			mismatch = true
		}
	}

	writeSuccess(w, memoryStatsData{
		DocumentCount: totalDocs,
		Initialized:   true,
		EntryCount:    totalDocs,
		SyncMismatch:  mismatch,
		TextCount:     memory.Count(memory.TypeText),
		ImageCount:    memory.Count(memory.TypeImage),
		AudioCount:    memory.Count(memory.TypeAudio),
	})
}

// handleMemoryListCollections GET /memory/collections — 列出全部分组集合
func handleMemoryListCollections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	names := memory.Collections()
	infos := make([]memoryCollectionInfo, 0, len(names))
	for _, name := range names {
		info := memory.CollectionInfo(name)
		if info == nil {
			continue
		}
		infos = append(infos, memoryCollectionInfo{
			Name:                   name,
			Dimension:              getIntField(info, "embedding_dimension"),
			Count:                  getIntField(info, "document_count"),
			Version:                getIntField(info, "version"),
			HasNameEmbedding:       getBoolField(info, "has_name_embedding"),
			NameEmbeddingDimension: getIntField(info, "name_embedding_dimension"),
			NameEmbeddingModel:     getStringField(info, "name_embedding_model"),
		})
	}

	writeSuccess(w, map[string]any{
		"collections": infos,
		"total":       len(infos),
	})
}

// handleMemoryCollectionStats GET /memory/{name}/stats — 集合统计
func handleMemoryCollectionStats(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	info := memory.CollectionInfo(collectionName)
	if info == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("记忆库请求[ERROR] -> 集合 '%s' 不存在", collectionName))
		return
	}

	count := getIntField(info, "document_count")
	mismatch := memory.HasSyncMismatch(collectionName)

	writeSuccess(w, memoryStatsData{
		DocumentCount: count,
		Initialized:   true,
		EntryCount:    count,
		SyncMismatch:  mismatch,
	})
}

// handleMemoryMessages POST/GET/DELETE /memory/messages — 全局类型化读写
