package lunar

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
)

// 源文件丢失/损坏时的固定提示
const lostFileNotice = "文件损毁或丢失,请更换句柄"

// 句柄不存在 / 无法解析时的固定提示
const handleInvalidNotice = "句柄无效,文件不存在"

// admitResource 统一入库：归一来源 → 落盘 → 登记资源目录 → 返回句柄
func admitResource(input persistence.AdmitInput) *persistence.AdmitResult {
	result, err := persistence.ResourceAdmit(input)
	if err != nil {
		logger.Error(lunarCoreModule, "[资源] 入库失败: %v", err)
		return nil
	}
	return &result
}

// admitDataURI 入库 data URI（工具产出、内联图片）
func admitDataURI(dataURI, name, origin string) *persistence.AdmitResult {
	return admitResource(persistence.AdmitInput{
		SourceKind: "data",
		Source:     dataURI,
		Name:       name,
		Origin:     origin,
	})
}

// admitImageDataURI 入库图片来源：同类型同内容哈希复用既有资源
//
// 来源形态不止 data URI：前端可能给 /file/read/ 路径，QQ 渠道可能给远程图床地址。
// 一律按取值推断来源类型，避免把 URL 当 data URI 解码而整张图入库失败。
func admitImageDataURI(source, name string) *persistence.AdmitResult {
	if source == "" {
		return nil
	}
	lower := strings.ToLower(source)
	sourceKind := "path"
	switch {
	case strings.HasPrefix(lower, "data:"):
		sourceKind = "data"
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		sourceKind = "url"
	}
	return admitResource(persistence.AdmitInput{
		SourceKind: sourceKind,
		Source:     source,
		Name:       name,
		KindHint:   "I",
		Origin:     "client-upload",
		Dedup:      true,
	})
}

// admitMedia 把媒体收进资源目录
func admitMedia(source, name, origin, kindHint string) *persistence.AdmitResult {
	if source == "" {
		return nil
	}
	lower := strings.ToLower(source)
	sourceKind := "path"
	if strings.HasPrefix(lower, "data:") {
		sourceKind = "data"
	} else if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		sourceKind = "url"
	}
	return admitResource(persistence.AdmitInput{
		SourceKind: sourceKind,
		Source:     source,
		Name:       name,
		KindHint:   kindHint,
		Origin:     origin,
	})
}

// readResource 按句柄取回资源内容（图片一律改走视觉副本）
func readResource(handle string, maxBytes int) *ResourceReadResult {
	parsed := parseHandle(handle)
	if parsed == nil {
		logger.Warn(lunarCoreModule, "[资源] 非法句柄: %s", handle)
		return nil
	}
	normalized := formatHandle(*parsed)
	if maxBytes <= 0 {
		maxBytes = persistence.MaxReadBytes
	}
	result, err := persistence.ResourceRead(normalized, maxBytes)
	if err != nil {
		logger.Warn(lunarCoreModule, "[资源] 取回失败（%s）: %v", normalized, err)
		return nil
	}
	if result.Lost {
		_ = persistence.ResourceDelete(normalized)
		return &result
	}
	if result.Kind == "I" && result.BlobPath != "" {
		data, mime, width, height, visionErr := visionCopy(result.BlobPath, result.Handle, 0)
		if visionErr != nil {
			if res, resolveErr := persistence.ResourceResolve(result.Handle); resolveErr == nil {
				_ = persistence.ResourceMarkLost(res.ID)
			}
			result.Lost = true
			result.Base64 = ""
			return &result
		}
		result.Base64 = base64.StdEncoding.EncodeToString(data)
		result.MIME = mime
		result.Truncated = false
		result.Width = width
		result.Height = height
	}
	return &result
}

// outlineResource 读取资源大纲（段落表）
func outlineResource(handle string) []ResourceSection {
	parsed := parseHandle(handle)
	if parsed == nil {
		return nil
	}
	sections, _, err := persistence.ResourceOutline(formatHandle(*parsed))
	if err != nil {
		return nil
	}
	return sections
}

// listResources 列出资源目录
func listResources(kind string, limit int) []ResourceEntry {
	if limit <= 0 {
		limit = 50
	}
	entries, err := persistence.ResourceList(kind, limit)
	if err != nil {
		logger.Warn(lunarCoreModule, "[资源] 列目录失败: %v", err)
		return nil
	}
	return entries
}

// searchResources 跨资源搜索
func searchResources(query, kind string, limit int) []ResourceSearchHit {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}
	hits, err := persistence.ResourceSearch(query, kind, limit)
	if err != nil {
		logger.Warn(lunarCoreModule, "[资源] 搜索失败: %v", err)
		return nil
	}
	return hits
}

// resourceStatistics 资源目录统计
func resourceStatistics() *ResourceStats {
	stats, err := persistence.ResourceStats()
	if err != nil {
		return nil
	}
	return &stats
}

// setResourceSummary 写入资源摘要/描述
func setResourceSummary(id int64, summary string, tags []string) {
	if err := persistence.ResourceSetSummary(id, summary, tags); err != nil {
		logger.Warn(lunarCoreModule, "[资源] 写摘要失败: %v", err)
	}
}

// deleteResource 显式删除资源
func deleteResource(handle string) bool {
	parsed := parseHandle(handle)
	if parsed == nil {
		return false
	}
	if err := persistence.ResourceDelete(formatHandle(*parsed)); err != nil {
		logger.Warn(lunarCoreModule, "[资源] 删除失败: %v", err)
		return false
	}
	return true
}

// getResourceEntry 按句柄取资源目录条目
func getResourceEntry(handle string) *ResourceEntry {
	parsed := parseHandle(handle)
	if parsed == nil {
		return nil
	}
	entries := listResources(parsed.Kind, 1000)
	for i := range entries {
		if int(entries[i].Seq) == parsed.Seq {
			return &entries[i]
		}
	}
	return nil
}

// resourceSource 取资源本体的相对 LocalDir 路径（仅 Go 侧管线使用）
func resourceSource(handle string) string {
	entry := getResourceEntry(handle)
	if entry == nil || entry.State != "active" || entry.BlobPath == "" {
		return ""
	}
	return entry.BlobPath
}

// resolveImageRef 把图片句柄解析为可取用来源
func resolveImageRef(raw string) (handle, source, desc string, ok bool) {
	parsed := parseHandle(raw)
	if parsed == nil || parsed.Kind != "I" {
		return "", "", "", false
	}
	normalized := formatHandle(*parsed)
	entry := getResourceEntry(normalized)
	if entry == nil {
		return "", "", "", false
	}
	if entry.State == "lost" {
		deleteResource(normalized)
		return "", "", "", false
	}
	if entry.State == "active" && entry.BlobPath != "" {
		source = entry.BlobPath
	}
	return normalized, source, entry.Summary, true
}

// degradeNotice 把资源不存在 / blob 丢失翻译为模型可见说明
func degradeNotice(handle, reason string) string {
	if parsed := parseHandle(handle); parsed != nil {
		handle = formatHandle(*parsed)
	}
	return fmt.Sprintf("%s（%s）", handle, reason)
}

// persistenceSaveBytes 以字节写入文件
func persistenceSaveBytes(name string, data []byte) (string, error) {
	result, err := persistence.SaveFileEx(name, persistence.SaveOptions{Overwrite: true}, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	return result.RelPath, nil
}
