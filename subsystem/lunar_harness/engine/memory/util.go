package memory

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// =============================================================================
// UUID v4 生成 — 对应前端 TypeScript UUID() 实现
// 遵循 RFC 4122 版本 4（随机 UUID），格式 xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
// =============================================================================

// generateUUID 生成一个符合 RFC 4122 v4 标准的 UUID 字符串
func generateUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		logger.Warn("FileManager", "crypto/rand 失败, 退化为时间UUID: %v", err)
		t := time.Now().UnixNano()
		for i := 0; i < 16; i++ {
			b[i] = byte(t >> (i * 4))
		}
	}
	b[6] = (b[6] & 0x0f) | 0x40 // 版本 4
	b[8] = (b[8] & 0x3f) | 0x80 // 变体 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// =============================================================================
// 余弦相似度
// =============================================================================

// cosineSimilarity 计算两个向量的余弦相似度
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}

	return float32(dotProduct / denom)
}

// =============================================================================
// JSON 文件 I/O 工具
// =============================================================================

// readJSONFile 读取 JSON 文件并反序列化到目标结构
func readJSONFile(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// atomicWriteJSON 原子写入 JSON 文件（先写临时文件，再重命名；单行紧凑存储，不格式化）
func atomicWriteJSON(path string, data interface{}) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("JSON 序列化失败: %w", err)
	}

	// 确保目标目录存在：集合目录可能被外部清理或重建遗漏，缺失时自动创建，避免写临时文件失败
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("确保目录存在失败: %w", err)
		}
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, jsonBytes, 0644); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	// Windows 上目标文件（尤其高频写的 metadata.json）可能被瞬时占用（并发读/AV 扫描/连续 rename），
	// 直接 rename 会偶发 "Access is denied"。先短暂重试，再尝试移除目标后重命名
	if err := substituteRename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("重命名临时文件失败: %w", err)
	}

	return nil
}

// substituteRename 替换式重命名：短暂重试 + 移除目标兜底，规避 Windows 目标文件占用导致 Access denied
func substituteRename(tmpPath, path string) error {
	for attempt := 0; attempt < 5; attempt++ {
		if err := os.Rename(tmpPath, path); err == nil {
			return nil
		}
		time.Sleep(30 * time.Millisecond)
	}
	// 兜底：先移除旧目标再重命名
	_ = os.Remove(path)
	if err := os.Rename(tmpPath, path); err == nil {
		return nil
	}
	return fmt.Errorf("rename %s %s", tmpPath, path)
}
