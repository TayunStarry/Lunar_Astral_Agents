package engine

import (
	"CorePersonality/personality/lunar/imagecache"
	"fmt"
	"strings"

	"github.com/dop251/goja"
)

// imagecache.go 图片缓存适配器 — 桥接 goja JS 运行时与共享图片缓存（imagecache 包）
//
// 对话者上下文中的每张静态图片在入口处注册进缓存并获得 [图片.00x] 标签；
// QQ 合并转发（Go 侧）直接写同一缓存。模型经 recall_image / select_reference_image
// 等工具按标签取回真实图片，图片字节从不随消息在模型上下文中搬运

// imageCachePut 注册图片来源并分配标签
// 参数: source（http(s) URL / data URI / 本地路径）, pinned（是否钉住防淘汰）
// 返回值: [tag, error] 图片标签（形如 图片.003）和错误信息
func (class *Runtime) imageCachePut(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("参数不足, 需 1 个: source")})
	}
	source, _ := call.Argument(0).Export().(string)
	if strings.TrimSpace(source) == "" {
		return class.runtime.ToValue([]any{"", fmt.Errorf("图片来源为空")})
	}
	pinned := false
	if len(call.Arguments) > 1 {
		if b, ok := call.Argument(1).Export().(bool); ok {
			pinned = b
		}
	}
	return class.runtime.ToValue([]any{imagecache.Put(source, pinned), nil})
}

// imageCacheGet 按标签查询缓存条目
// 参数: tag（形如 图片.003）
// 返回值: [{tag, source, desc, pinned, timestamp}, error] 条目不存在时首元素为 null
func (class *Runtime) imageCacheGet(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("参数不足, 需 1 个: tag")})
	}
	tag, _ := call.Argument(0).Export().(string)
	entry, ok := imagecache.Get(tag)
	if !ok {
		return class.runtime.ToValue([]any{nil, nil})
	}
	return class.runtime.ToValue([]any{map[string]any{
		"tag":       entry.Tag,
		"source":    entry.Source,
		"desc":      entry.Desc,
		"pinned":    entry.Pinned,
		"timestamp": entry.Timestamp,
	}, nil})
}

// imageCacheList 按注册序号倒序返回最近 topK 个条目
// 参数: topK
// 返回值: [Array<{tag, source, desc, pinned, timestamp}>, error]
func (class *Runtime) imageCacheList(call goja.FunctionCall) goja.Value {
	topK := int(call.Argument(0).ToInteger())
	entries := imagecache.List(topK)
	result := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		result = append(result, map[string]any{
			"tag":       entry.Tag,
			"source":    entry.Source,
			"desc":      entry.Desc,
			"pinned":    entry.Pinned,
			"timestamp": entry.Timestamp,
		})
	}
	return class.runtime.ToValue([]any{result, nil})
}

// imageCacheSetDesc 写入条目内容描述
// 参数: tag, desc
// 返回值: [boolean, error] 是否成功
func (class *Runtime) imageCacheSetDesc(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return class.runtime.ToValue([]any{false, fmt.Errorf("参数不足, 需 2 个: tag, desc")})
	}
	tag, _ := call.Argument(0).Export().(string)
	desc, _ := call.Argument(1).Export().(string)
	imagecache.SetDesc(tag, desc)
	return class.runtime.ToValue([]any{true, nil})
}

// imageCacheUnpin 解除条目钉住（图片降级为占位标签后调用）
// 参数: tag
// 返回值: [boolean, error] 是否成功
func (class *Runtime) imageCacheUnpin(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{false, fmt.Errorf("参数不足, 需 1 个: tag")})
	}
	tag, _ := call.Argument(0).Export().(string)
	imagecache.Unpin(tag)
	return class.runtime.ToValue([]any{true, nil})
}
