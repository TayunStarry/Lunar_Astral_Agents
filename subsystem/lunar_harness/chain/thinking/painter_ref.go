package thinking

// painter_ref.go 把创作链的参考资源解析为图片绘制智能体可直接使用的本地图片。
//
// 支持三类来源：
//   - 图片 / 动图句柄（@I.003、@G.004，锚点会被忽略）；
//   - 文档片段（@D.020|§1、@D.020|L10-20），从片段里提取内嵌的图片句柄或本机图片路径；
//   - /file/read/ 或 multimedia/ 形式的本机图片路径。
//
// 解析失败必须返回明确原因，禁止静默退化成「没有参考图」。

import (
	"fmt"
	"regexp"
	"strings"

	"LunarSubsystem/LunarHarness/agents/Painter"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// painterRef 一张已定位到本体的参考图。
type painterRef struct {
	Handle string
	Source string
	Desc   string
}

// imagePathRegexp 从文档片段里兜底提取本机图片路径。
var imagePathRegexp = regexp.MustCompile(`(?i)(?:/file/(?:read|miniature)/)?multimedia/[^\s"'()\[\]<>]+\.(?:png|jpe?g|webp|gif|bmp)`)

// PainterReferences 解析创作链传入的参考资源列表。
//
// 多个来源会按顺序去重；第一张作为图生图基座，其余作为视觉对照。
func PainterReferences(raws []string) ([]Painter.Reference, error) {
	out := make([]Painter.Reference, 0, len(raws))
	seen := make(map[string]bool, len(raws))
	for _, raw := range raws {
		refs, err := resolvePainterRefs(raw)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			key := ref.Handle
			if key == "" {
				key = ref.Source
			}
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, Painter.Reference{Handle: ref.Handle, Source: ref.Source, Desc: ref.Desc})
			if len(out) >= maxPainterReferences {
				return out, nil
			}
		}
	}
	return out, nil
}

// resolveImageRef 把单个图片句柄解析为可取用来源，供人格对话的绘画调度使用。
//
// 与创作链的严格解析不同：这里保留「句柄存在但本体已回收」的中间态，
// 让对话者能提示用户重新发送图片。
func resolveImageRef(raw string) (handle, source, desc string, ok bool) {
	parsed := parseHandle(raw)
	if parsed == nil || (parsed.Kind != resource.KindImage && parsed.Kind != resource.KindAnimated) {
		return "", "", "", false
	}
	parsed.Anchor = "none"
	normalized := formatHandle(*parsed)
	entry := getResourceEntry(normalized)
	if entry == nil {
		return "", "", "", false
	}
	if entry.State == resource.StateLost {
		deleteResource(normalized)
		return "", "", "", false
	}
	if entry.State == resource.StateActive {
		source = entry.BlobPath
	}
	return normalized, source, entry.Summary, true
}

// resolvePainterRefs 解析单个参考资源；空串视为「没有参考图」。
func resolvePainterRefs(raw string) ([]painterRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if handle := parseHandle(raw); handle != nil {
		switch handle.Kind {
		case resource.KindImage, resource.KindAnimated:
			return resolveImageHandle(*handle)
		case resource.KindDocument:
			return resolveDocumentRefs(raw)
		default:
			return nil, fmt.Errorf("参考资源 %s 不是图片（%s），不能作为图生图基座", raw, resource.KindLabel(handle.Kind))
		}
	}
	ref, err := resolveImagePath(raw)
	if err != nil {
		return nil, fmt.Errorf("参考资源 %q 不是可识别的图片句柄或本地图片路径", raw)
	}
	return []painterRef{ref}, nil
}

// resolveImageHandle 解析图片 / 动图句柄；锚点不影响取本体。
func resolveImageHandle(handle ResourceHandle) ([]painterRef, error) {
	handle.Anchor = "none"
	normalized := formatHandle(handle)
	entry := getResourceEntry(normalized)
	if entry == nil {
		return nil, fmt.Errorf("参考图 %s 不存在", normalized)
	}
	if entry.State == resource.StateLost {
		deleteResource(normalized)
		return nil, fmt.Errorf("参考图 %s 的本体已回收，请改用其它图片或重新上传", normalized)
	}
	if entry.State != resource.StateActive || entry.BlobPath == "" {
		return nil, fmt.Errorf("参考图 %s 当前不可用（状态：%s）", normalized, entry.State)
	}
	return []painterRef{{Handle: normalized, Source: entry.BlobPath, Desc: entry.Summary}}, nil
}

// resolveDocumentRefs 从文档片段中提取图片：优先内嵌句柄，其次本机图片路径。
func resolveDocumentRefs(raw string) ([]painterRef, error) {
	result, err := resource.Read(raw, maxPainterRefDocBytes)
	if err != nil {
		return nil, fmt.Errorf("读取参考文档 %s 失败：%w", raw, err)
	}
	if result.Lost || strings.TrimSpace(result.Content) == "" {
		return nil, fmt.Errorf("参考文档 %s 没有可读内容", raw)
	}
	out := make([]painterRef, 0, maxPainterReferences)
	for _, text := range resource.ExtractHandles(result.Content) {
		handle := parseHandle(text)
		if handle == nil || (handle.Kind != resource.KindImage && handle.Kind != resource.KindAnimated) {
			continue
		}
		refs, err := resolveImageHandle(*handle)
		if err != nil {
			continue
		}
		out = append(out, refs...)
		if len(out) >= maxPainterReferences {
			return out, nil
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	for _, path := range imagePathRegexp.FindAllString(result.Content, -1) {
		ref, err := resolveImagePath(path)
		if err != nil {
			continue
		}
		out = append(out, ref)
		if len(out) >= maxPainterReferences {
			return out, nil
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("参考文档 %s 的指定片段里没有可用的图片句柄或本地图片路径", raw)
	}
	return out, nil
}

// resolveImagePath 把本机图片路径解析为参考图；已登记的资源会一并带上句柄。
func resolveImagePath(raw string) (painterRef, error) {
	rel := imageRelOf(raw)
	if rel == "" {
		return painterRef{}, fmt.Errorf("不是本机图片路径")
	}
	ext := rel
	if index := strings.LastIndex(ext, "."); index >= 0 {
		ext = ext[index:]
	} else {
		ext = ""
	}
	if resource.KindOfExt(ext) != resource.KindImage {
		return painterRef{}, fmt.Errorf("不是图片扩展名")
	}
	if entry, ok := resource.ForPath(rel); ok {
		if entry.State != resource.StateActive || entry.BlobPath == "" {
			return painterRef{}, fmt.Errorf("图片资源 %s 当前不可用", rel)
		}
		return painterRef{Handle: entry.Handle, Source: entry.BlobPath, Desc: entry.Summary}, nil
	}
	return painterRef{Source: rel}, nil
}

// imageRelOf 把 /file/read/ 地址或本地绝对路径归一化为 multimedia/ 相对路径。
func imageRelOf(raw string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	if cut := strings.IndexAny(normalized, "?#"); cut >= 0 {
		normalized = normalized[:cut]
	}
	lower := strings.ToLower(normalized)
	if marker := strings.LastIndex(lower, "/multimedia/"); marker >= 0 {
		return normalized[marker+1:]
	}
	if strings.HasPrefix(lower, "multimedia/") {
		return normalized
	}
	return ""
}
