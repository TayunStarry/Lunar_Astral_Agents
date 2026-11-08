package thinking

import (
	"bytes"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// admitResource 统一入库：归一来源 → 落盘 → 登记资源目录 → 返回句柄
//
// 图片入库失败属于可预期的资源缺失（历史消息里的旧地址、渠道临时文件等），
// 按系统规范直接跳过并销毁可能残留的句柄，不把错误级别抛到日志面板。
func admitResource(input resource.AdmitInput) (*resource.AdmitResult, error) {
	result, err := resource.Admit(input)
	if err != nil {
		if input.KindHint == resource.KindImage || input.KindHint == resource.KindAnimated {
			forgetResourceSource(input.Source)
			logger.Info(lunarCoreModule, "[资源] 图片入库失败，已跳过并销毁句柄（%s）: %v", resource.ShortURLForLog(input.Source), err)
		} else {
			logger.Error(lunarCoreModule, "[资源] 入库失败: %v", err)
		}
		return nil, err
	}
	return &result, nil
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

// classifySourceKind 来源形态 → 分类层的来源类型；只做形态归一，不参与类型判定
func classifySourceKind(source string) string {
	trimmed := strings.TrimSpace(source)
	switch {
	case strings.HasPrefix(strings.ToLower(trimmed), "data:"):
		return resource.SourceKindData
	case resource.IsRemoteURL(trimmed):
		return resource.SourceKindURL
	default:
		return resource.SourceKindPath
	}
}

// admitImageDataURI 入库图片来源：同类型同内容哈希复用既有资源
//
// 来源形态不止 data URI：前端可能给 /file/read/ 路径，QQ 渠道可能给远程图床地址。
// 一律按取值推断来源类型，避免把 URL 当 data URI 解码而整张图入库失败。
// 媒体种类由调用方按消息段 / 分类结果传入，这里不按链接后缀改判。
func admitImageDataURI(source, name, kindHint string) *resource.AdmitResult {
	result, err := admitImageSource(source, name, kindHint)
	if err != nil {
		return nil
	}
	return result
}

// admitImageSource 入库图片并保留失败原因。
//
// 无论来源是 data URI、本机路径还是 QQ 图床直链，媒体种类都由调用方按分类层
// 给出的类型码传入，这里只负责下载与登记，不根据链接后缀改判媒体类型。
func admitImageSource(source, name, kindHint string) (*resource.AdmitResult, error) {
	if source == "" {
		return nil, fmt.Errorf("图片地址为空")
	}
	// 腾讯图床只接受 https：历史消息里的 http 直链会直接 400，入库前统一升级。
	source = resource.UpgradeCDNScheme(source)
	return admitResource(resource.AdmitInput{
		SourceKind: classifySourceKind(source),
		Source:     source,
		Name:       name,
		KindHint:   kindHint,
		Origin:     "client-upload",
		Dedup:      true,
	})
}

// admitMedia 把媒体收进资源目录
func admitMedia(source, name, origin, kindHint string) *resource.AdmitResult {
	if source == "" {
		return nil
	}
	result, err := admitResource(resource.AdmitInput{
		SourceKind: classifySourceKind(source),
		Source:     source,
		Name:       name,
		KindHint:   kindHint,
		Origin:     origin,
	})
	if err != nil {
		return nil
	}
	return result
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

// persistenceSaveBytes 以字节写入文件
func persistenceSaveBytes(name string, data []byte) (string, error) {
	result, err := persistence.SaveFileEx(name, persistence.SaveOptions{Overwrite: true}, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	return result.RelPath, nil
}
