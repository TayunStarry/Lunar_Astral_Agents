package memory

// layout_write.go 目录分片写入

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func writeStoreDir(dir string, dimension int, docs []Document) (int, error) {
	return writeClusterDirCommon(dir, dimension, docs)
}

// writeClusterDir 把一个正式簇的全部条目写成统一分片
func writeClusterDir(dir string, dimension int, docs []Document) (int, error) {
	return writeClusterDirCommon(dir, dimension, docs)
}

func writeClusterDirCommon(dir string, dimension int, docs []Document) (int, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, err
	}
	docs = sortedDocsByTime(docs)
	chunkSize := DocumentsChunkSize
	chunks := (len(docs) + chunkSize - 1) / chunkSize
	if len(docs) == 0 {
		chunks = 0
	}
	written := 0
	for i := 0; i < chunks; i++ {
		chunkNum := i + 1
		start := i * chunkSize
		end := start + chunkSize
		if end > len(docs) {
			end = len(docs)
		}
		docChunk := docs[start:end]
		if err := atomicWriteJSON(filepath.Join(dir, fmt.Sprintf("documents_%04d.json", chunkNum)), docChunk); err != nil {
			return written, err
		}
		embChunk := make([]embeddingRecord, len(docChunk))
		for j := range docChunk {
			embChunk[j] = embeddingRecord{ID: docChunk[j].ID, Embedding: docChunk[j].Embedding}
		}
		if err := atomicWriteJSON(filepath.Join(dir, fmt.Sprintf("embeddings_%04d.json", chunkNum)), embChunk); err != nil {
			return written, err
		}
		written += 2
	}
	removeChunksBeyond(dir, chunks)
	meta := collectionMeta{
		EmbeddingDimension:   dimension,
		Version:              CurrentVersion,
		DocumentsChunkCount:  chunks,
		EmbeddingsChunkCount: chunks,
	}
	if err := atomicWriteJSON(filepath.Join(dir, "metadata.json"), meta); err != nil {
		return written, err
	}
	return written, nil
}

// removeDocsFromDir 从目录分片里按 ID 移除条目（保持分片结构）
func removeDocsFromDir(dir string, idsToRemove []string) error {
	removeSet := make(map[string]bool, len(idsToRemove))
	for _, id := range idsToRemove {
		removeSet[id] = true
	}
	if len(removeSet) == 0 {
		return nil
	}

	docs, _, _, err := readDocumentsFromDir(dir)
	if err != nil {
		return err
	}
	filtered := docs[:0]
	for _, doc := range docs {
		if removeSet[doc.ID] {
			continue
		}
		filtered = append(filtered, doc)
	}
	if len(filtered) == len(docs) {
		return nil // 没有要删的
	}
	_, err = writeClusterDirCommon(dir, len(docs[0].Embedding), filtered)
	return err
}

// removeChunksBeyond 删除编号超过 chunks 的旧分片
func removeChunksBeyond(dir string, chunks int) {
	for _, pattern := range []string{"documents_*.json", "embeddings_*.json"} {
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))
		for _, path := range matches {
			base := filepath.Base(path)
			ext := filepath.Ext(base)
			parts := strings.SplitN(base, "_", 2)
			if len(parts) < 2 {
				continue
			}
			numPart := strings.TrimSuffix(parts[1], ext)
			num, err := strconv.Atoi(numPart)
			if err == nil && num > chunks {
				os.Remove(path)
			}
		}
	}
}

// reconcileClusterLayout 逐项对账：总库条目 + 正式簇条目 = 原始总数
