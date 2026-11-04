package thinking

import (
	"bytes"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// admitResource 统一入库：归一来源 → 落盘 → 登记资源目录 → 返回句柄
//
// 图片入库失败属于可预期的资源缺失（历史消息里的旧地址、渠道临时文件等），
// 按系统规范直接跳过并销毁可能残留的句柄，不把错误级别抛到日志面板。
func admitResource(input resource.AdmitInput) *resource.AdmitResult {
	result, err := resource.Admit(input)
	if err != nil {
		if input.KindHint == "I" {
			forgetResourceSource(input.Source)
			logger.Info(lunarCoreModule, "[资源] 图片入库失败，已跳过并销毁句柄（%s）: %v", resource.ShortURLForLog(input.Source), err)
		} else {
			logger.Error(lunarCoreModule, "[资源] 入库失败: %v", err)
		}
		return nil
	}
	return &result
}

// forgetResourceSource 资源入库失败时销毁来源对应的句柄映射。
//
// 接受两类来源：已经是 @X.nnn 句柄，或 /file/read/<rel>、multimedia/<rel> 形式的本机路径。
// 只清句柄与目录行，不删除可能仍被其它资源引用的物理实体。
func forgetResourceSource(source string) {
	raw := strings.TrimSpace(source)
	if raw == "" {
		return
	}
	if parsed := parseHandle(raw); parsed != nil {
		_ = resource.Delete(formatHandle(*parsed))
		return
	}
	rel := ""
	if index := strings.Index(raw, fileReadPath); index >= 0 {
		rel = raw[index+len(fileReadPath):]
	} else if strings.HasPrefix(raw, "multimedia/") || strings.HasPrefix(raw, "audios/") {
		rel = raw
	}
	rel = strings.TrimPrefix(strings.TrimSpace(rel), "/")
	if rel != "" {
		resource.ForgetPath(rel)
	}
}

// admitImageDataURI 入库图片来源：同类型同内容哈希复用既有资源
//
// 来源形态不止 data URI：前端可能给 /file/read/ 路径，QQ 渠道可能给远程图床地址。
// 一律按取值推断来源类型，避免把 URL 当 data URI 解码而整张图入库失败。
func admitImageDataURI(source, name string) *resource.AdmitResult {
	if source == "" {
		return nil
	}
	// 腾讯图床只接受 https：历史消息里的 http 直链会直接 400，入库前统一升级。
	source = resource.UpgradeCDNScheme(source)
	lower := strings.ToLower(source)
	sourceKind := "path"
	switch {
	case strings.HasPrefix(lower, "data:"):
		sourceKind = "data"
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		sourceKind = "url"
	}
	return admitResource(resource.AdmitInput{
		SourceKind: sourceKind,
		Source:     source,
		Name:       name,
		KindHint:   "I",
		Origin:     "client-upload",
		Dedup:      true,
	})
}

// admitMedia 把媒体收进资源目录
func admitMedia(source, name, origin, kindHint string) *resource.AdmitResult {
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
	return admitResource(resource.AdmitInput{
		SourceKind: sourceKind,
		Source:     source,
		Name:       name,
		KindHint:   kindHint,
		Origin:     origin,
	})
}

// deleteResource 显式删除资源
func deleteResource(handle string) bool {
	parsed := parseHandle(handle)
	if parsed == nil {
		return false
	}
	if err := resource.Delete(formatHandle(*parsed)); err != nil {
		logger.Warn(lunarCoreModule, "[资源] 删除失败: %v", err)
		return false
	}
	return true
}

// getResourceEntry 按句柄取资源记录（句柄表点查，O(1)）
func getResourceEntry(handle string) *ResourceEntry {
	entry, ok := resource.Lookup(handle)
	if !ok {
		return nil
	}
	return &entry
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

// persistenceSaveBytes 以字节写入文件
func persistenceSaveBytes(name string, data []byte) (string, error) {
	result, err := persistence.SaveFileEx(name, persistence.SaveOptions{Overwrite: true}, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	return result.RelPath, nil
}
