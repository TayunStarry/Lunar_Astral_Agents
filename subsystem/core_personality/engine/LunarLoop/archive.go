package lunarloop

// archive.go 统一资源归档层的 goja 适配器
//
// 桥接 TypeScript 与共享子系统 FileManager/module 的资源目录（archive.db）、
// 统一入库入口（ResourceAdmit）与内容取回（ResourceRead）。
// JS 侧签名（唯一权威定义见 globals.d.ts）：
//   resourceAdmit(input)              → [result, error]  入参 {sourceKind, source, data, name, kindHint, origin, retain, mime}
//   resourceRead(handle, maxBytes)    → [result, error]
//   resourceOutline(handle)           → [sections, handle, error]
//   resourceList(kind, limit)         → [resources, error]
//   resourceSearch(query, kind, limit)→ [hits, error]
//   resourceStats()                   → [stats, error]
//   resourceSetSummary(id, summary, tags) → [bool, error]
//   resourceSetSections(id, sections)→ [bool, error]
//   resourceCollect(force)            → [result, error]
//   resourceDelete(handle)            → [bool, error]
//   resourceGrammar()                 → {pattern, handle, section, line, typeCodes}

import (
	"LunarSubsystem/FileManager/module"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/dop251/goja"
)

// toJS 把 Go 结构体转换为 JS 可见的值，**严格遵循结构的 json 标签**
//
// 为什么必须这样做：goja 直接返回结构体时按 Go 字段名（Handle / ContentHash / …）
// 映射为 JS 属性，而 TypeScript 侧的类型定义与全部消费代码用的是 json 标签名
// （handle / content_hash / …）。不转换的话 JS 侧读到的一律是 undefined，
// 表现为「句柄为空」「段落表写不进去」这类静默失败。
//
// 走一次 JSON 往返是最省事且不会漂移的做法：json 标签成为唯一的线上字段名来源。
// 对集合类型的调用方应优先整体转换一次（本函数也支持切片与 map）。
func toJS(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		// 序列化失败不应让整个调用崩掉：交由调用方按空结果处理
		return nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	// 保证对象形态（nil 切片序列化为 null，与 TS 侧的 null 判定一致）
	return out
}

// exportMap 把 goja 对象导出为 map（非对象返回 nil）
func exportMap(v goja.Value) map[string]any {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	exported := v.Export()
	if m, ok := exported.(map[string]any); ok {
		return m
	}
	return nil
}

// mapString 从 map 取字符串字段
func mapString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

// mapInt 从 map 取整数字段（兼容 goja 的 float64）
func mapInt(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int64:
		return int(v)
	case int:
		return v
	}
	return 0
}

// mapBool 从 map 取布尔字段
func mapBool(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	if b, ok := m[key].(bool); ok {
		return b
	}
	return false
}

// decodeMapBytes 从导出后的入参 map 中解码字节字段（兼容 []byte 与 base64 字符串）
func decodeMapBytes(m map[string]any, key string) ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	switch v := m[key].(type) {
	case nil:
		return nil, nil
	case []byte:
		return v, nil
	case string:
		if v == "" {
			return nil, nil
		}
		return base64.StdEncoding.DecodeString(v)
	default:
		return nil, fmt.Errorf("字节入参类型不受支持: %T", v)
	}
}

// resourceAdmit 统一入库入口：归一来源 → 落盘 → 登记资源目录 → 返回句柄
// 参数: input {sourceKind, source, data(base64), name, kindHint, origin, retain, mime}
// 返回值: [result, error]
func (class *Runtime) resourceAdmit(call goja.FunctionCall) goja.Value {
	in := exportMap(call.Argument(0))
	if in == nil {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("resourceAdmit 需提供输入对象")})
	}
	data, err := decodeMapBytes(in, "data")
	if err != nil {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("resourceAdmit 字节入参解码失败: %v", err)})
	}
	admit := module.AdmitInput{
		SourceKind: mapString(in, "sourceKind"),
		Source:     mapString(in, "source"),
		Data:       data,
		Name:       mapString(in, "name"),
		KindHint:   mapString(in, "kindHint"),
		Origin:     mapString(in, "origin"),
		Retain:     mapString(in, "retain"),
		MIME:       mapString(in, "mime"),
		Dedup:      mapBool(in, "dedup"),
	}
	// 入参完整性校验：来源与字节至少提供一个，否则一律拒绝。
	// （缺了这道闸，空对象会一路走到建表，写出 kind="" / seq=0 的垃圾目录行）
	if admit.SourceKind == "" {
		if len(admit.Data) == 0 && admit.Source == "" {
			return class.runtime.ToValue([]any{nil, fmt.Errorf("resourceAdmit 缺少 sourceKind，且未提供 source 或 data")})
		}
		// 未显式给来源类型时按取值推断，避免调用方漏传导致行为不确定
		if len(admit.Data) > 0 {
			admit.SourceKind = module.SourceKindBytes
		} else if len(admit.Source) > 0 {
			admit.SourceKind = module.SourceKindPath
		}
	}
	if admit.SourceKind != module.SourceKindBytes && admit.Source == "" {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("resourceAdmit 的 %s 来源缺少 source", admit.SourceKind)})
	}
	if admit.SourceKind == module.SourceKindBytes && len(admit.Data) == 0 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("resourceAdmit 的 bytes 来源缺少 data")})
	}
	result, err := module.ResourceAdmit(admit)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{toJS(result), nil})
}

// resourceRead 按句柄取回资源内容（带锚点时按锚点定位整段）
// 返回值: [result, error]
func (class *Runtime) resourceRead(call goja.FunctionCall) goja.Value {
	handle := call.Argument(0).String()
	maxBytes := int(call.Argument(1).ToInteger())
	if maxBytes <= 0 {
		maxBytes = module.MaxReadBytes
	}
	result, err := module.ResourceRead(handle, maxBytes)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{toJS(result), nil})
}

// resourceOutline 读取资源大纲（段落表）
// 返回值: [sections, canonicalHandle, error]
func (class *Runtime) resourceOutline(call goja.FunctionCall) goja.Value {
	handle := call.Argument(0).String()
	sections, canonical, err := module.ResourceOutline(handle)
	if err != nil {
		return class.runtime.ToValue([]any{nil, "", err})
	}
	return class.runtime.ToValue([]any{toJS(sections), canonical, nil})
}

// resourceList 列出资源目录
// 返回值: [resources, error]
func (class *Runtime) resourceList(call goja.FunctionCall) goja.Value {
	kind := call.Argument(0).String()
	limit := int(call.Argument(1).ToInteger())
	if limit <= 0 {
		limit = 50
	}
	resources, err := module.ResourceList(kind, limit)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{toJS(resources), nil})
}

// resourceSearch 跨资源搜索
// 返回值: [hits, error]
func (class *Runtime) resourceSearch(call goja.FunctionCall) goja.Value {
	query := call.Argument(0).String()
	kind := call.Argument(1).String()
	limit := int(call.Argument(2).ToInteger())
	if limit <= 0 {
		limit = 10
	}
	hits, err := module.ResourceSearch(query, kind, limit)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{toJS(hits), nil})
}

// resourceStats 资源目录统计
// 返回值: [stats, error]
func (class *Runtime) resourceStats(call goja.FunctionCall) goja.Value {
	stats, err := module.ResourceStats()
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{toJS(stats), nil})
}

// resourceSetSummary 写入资源摘要/描述（图片视觉描述、文档概要）；tags 传空数组表示保留原标签
// 返回值: [bool, error]
func (class *Runtime) resourceSetSummary(call goja.FunctionCall) goja.Value {
	id := call.Argument(0).ToInteger()
	summary := call.Argument(1).String()
	var tags []string
	if exported := call.Argument(2).Export(); exported != nil {
		if arr, ok := exported.([]any); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok && s != "" {
					tags = append(tags, s)
				}
			}
		}
	}
	if err := module.ResourceSetSummary(id, summary, tags); err != nil {
		return class.runtime.ToValue([]any{false, err})
	}
	return class.runtime.ToValue([]any{true, nil})
}

// resourceSetSections 全量替换资源段落表（入库与迁移重建使用）
// 入参: id, sections[{ordinal, level, title, path, lineStart, lineEnd, chars}]
// 返回值: [bool, error]
func (class *Runtime) resourceSetSections(call goja.FunctionCall) goja.Value {
	id := call.Argument(0).ToInteger()
	exported := call.Argument(1).Export()
	rawArr, ok := exported.([]any)
	if !ok {
		return class.runtime.ToValue([]any{false, fmt.Errorf("resourceSetSections 第 2 个参数需为段落数组")})
	}
	drafts := make([]module.ArchiveSectionDraft, 0, len(rawArr))
	for _, item := range rawArr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		drafts = append(drafts, module.ArchiveSectionDraft{
			Ordinal:   mapInt(m, "ordinal"),
			Level:     mapInt(m, "level"),
			Title:     mapString(m, "title"),
			Path:      mapString(m, "path"),
			LineStart: mapInt(m, "lineStart"),
			LineEnd:   mapInt(m, "lineEnd"),
			Chars:     mapInt(m, "chars"),
		})
	}
	if err := module.ResourceReplaceSections(id, drafts); err != nil {
		return class.runtime.ToValue([]any{false, err})
	}
	return class.runtime.ToValue([]any{true, nil})
}

// resourceCollect 触发内容仓库回收
// 返回值: [result, error]
func (class *Runtime) resourceCollect(call goja.FunctionCall) goja.Value {
	force := mapBool(exportMap(call.Argument(0)), "force")
	result, err := module.ArchiveCollect(force)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{toJS(result), nil})
}

// resourceDelete 显式删除资源（目录行 + 独占 blob）
// 返回值: [bool, error]
func (class *Runtime) resourceDelete(call goja.FunctionCall) goja.Value {
	if err := module.ResourceDelete(call.Argument(0).String()); err != nil {
		return class.runtime.ToValue([]any{false, err})
	}
	return class.runtime.ToValue([]any{true, nil})
}

// resourceGrammar 下发句柄文法（Go 侧为唯一权威定义，TS 与前端不得各写一套）
// 返回值: 文法对象
func (class *Runtime) resourceGrammar(call goja.FunctionCall) goja.Value {
	return class.runtime.ToValue(map[string]any{
		"pattern":  module.HandlePattern,
		"handle":   "@<类型码>.<序号>",
		"section":  "|§<章节序号>",
		"line":     "|L<起始行>-<结束行>",
		"seqWidth": module.SeqPadWidth,
		"typeCodes": map[string]string{
			module.KindDocument: "文档（文本类文件：源码 / Markdown / 纯文本 / 配置）",
			module.KindImage:    "图片",
			module.KindVideo:    "视频",
			module.KindAudio:    "音频",
			module.KindTable:    "表格",
		},
	})
}
