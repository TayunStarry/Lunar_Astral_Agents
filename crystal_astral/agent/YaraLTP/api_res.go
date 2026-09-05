package YaraLTP

// ==== 资源类 API：config / file / database / knowledge / image ====

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	file "LunarSubsystem/FileManager/module"
	"LunarSubsystem/LoggerGeneral"
	"github.com/dop251/goja"
)

// bindConfig 注入 yara.config（读写在 config.yaml，LTP3 唯一配置文件）。
func bindConfig(p *plugin, parent *goja.Object) {
	vm := p.vm
	o := newObj(vm)
	objSetFn(o, "getFile", func(call goja.FunctionCall) goja.Value {
		if p.config == nil {
			return vm.ToValue(goja.Undefined())
		}
		return vm.ToValue(p.config)
	})
	objSetFn(o, "setFile", func(call goja.FunctionCall) goja.Value {
		cfg := argMap(call, 0)
		data := marshalYAML(cfg)
		if err := os.WriteFile(p.ConfigPath, []byte(data), 0644); err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		p.config = cfg
		// 通知 onConfigUpdate（存在时）
		if p.onConfigUpdateFn != nil {
			if _, rerr := p.callFn(p.onConfigUpdateFn, "plugin", cfg, "1.0.0"); rerr != nil {
				LoggerGeneral.Warn(ServiceName, "插件 %s onConfigUpdate 异常: %v", p.ID, rerr)
			}
		}
		return vm.ToValue(true)
	})
	parent.Set("config", o)
}

// bindFile 注入 yara.file（/ 根目录与 data/ 运行时目录）。
func bindFile(p *plugin, parent *goja.Object) {
	vm := p.vm
	o := newObj(vm)
	objSetFn(o, "read", func(call goja.FunctionCall) goja.Value {
		path, err := safeResolve(p.Root, "plugin", argString(call, 0))
		if err != nil {
			return vm.ToValue(goja.Undefined())
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return vm.ToValue(goja.Undefined())
		}
		return vm.ToValue(string(b))
	})
	objSetFn(o, "write", func(call goja.FunctionCall) goja.Value {
		path, err := safeResolve(p.Root, "plugin", argString(call, 0))
		if err != nil {
			return vm.ToValue(false)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return vm.ToValue(false)
		}
		if err := os.WriteFile(path, []byte(argString(call, 1)), 0644); err != nil {
			return vm.ToValue(false)
		}
		return vm.ToValue(true)
	})
	objSetFn(o, "readData", func(call goja.FunctionCall) goja.Value {
		path, err := safeResolve(p.DataDir, "data", argString(call, 0))
		if err != nil {
			return vm.ToValue(goja.Undefined())
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return vm.ToValue(goja.Undefined())
		}
		return vm.ToValue(string(b))
	})
	objSetFn(o, "writeData", func(call goja.FunctionCall) goja.Value {
		path, err := safeResolve(p.DataDir, "data", argString(call, 0))
		if err != nil {
			return vm.ToValue(false)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return vm.ToValue(false)
		}
		if err := os.WriteFile(path, []byte(argString(call, 1)), 0644); err != nil {
			return vm.ToValue(false)
		}
		return vm.ToValue(true)
	})
	objSetFn(o, "listData", func(call goja.FunctionCall) goja.Value {
		rel := argString(call, 0)
		dir := p.DataDir
		if rel != "" {
			sub, err := safeResolve(p.DataDir, "data", rel)
			if err != nil {
				return vm.ToValue([]any{})
			}
			dir = sub
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return vm.ToValue([]any{})
		}
		out := make([]string, 0, len(entries))
		for _, e := range entries {
			out = append(out, e.Name())
		}
		return vm.ToValue(out)
	})
	objSetFn(o, "getDataPath", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(p.DataDir)
	})
	parent.Set("file", o)
}

// bindDatabase 注入 yara.database（复用当前项目记忆库基建）。
func bindDatabase(vm *goja.Runtime, parent *goja.Object) {
	o := newObj(vm)
	objSetFn(o, "queryMessages", func(call goja.FunctionCall) goja.Value {
		opts := argMap(call, 0)
		limit := int(toInt64(mapGet(opts, "limit")))
		if limit <= 0 || limit > 200 {
			limit = 20
		}
		entries, _, err := queryMessages(limit)
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(entries)
	})
	objSetFn(o, "searchMessages", func(call goja.FunctionCall) goja.Value {
		opts := argMap(call, 0)
		q := mapGetStr(opts, "query")
		limit := int(toInt64(mapGet(opts, "limit")))
		if limit <= 0 || limit > 200 {
			limit = 20
		}
		entries, total, err := searchMessages(q, limit)
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"entries": entries, "total": total})
	})
	objSetFn(o, "getUserMessages", func(call goja.FunctionCall) goja.Value {
		opts := argMap(call, 0)
		limit := int(toInt64(mapGet(opts, "limit")))
		if limit <= 0 || limit > 200 {
			limit = 20
		}
		entries, _, err := queryMessages(limit)
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(entries)
	})
	objSetFn(o, "getUserInfo", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(nil)
	})
	parent.Set("database", o)
}

// bindKnowledge 注入 yara.knowledge（复用记忆库向量检索）。
func bindKnowledge(vm *goja.Runtime, parent *goja.Object) {
	o := newObj(vm)
	objSetFn(o, "search", func(call goja.FunctionCall) goja.Value {
		opts := argMap(call, 0)
		q := mapGetStr(opts, "query")
		limit := int(toInt64(mapGet(opts, "limit")))
		if limit <= 0 {
			limit = 5
		}
		entries, total, err := searchMessages(q, limit)
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"entries": entries, "total": total})
	})
	parent.Set("knowledge", o)
}

// bindImage 注入 yara.image。
func bindImage(p *plugin, parent *goja.Object) {
	vm := p.vm
	o := newObj(vm)
	objSetFn(o, "getCached", func(call goja.FunctionCall) goja.Value {
		// 主程序图片缓存未接入，返回 null 由插件走 loadValid/URL 兜底。
		return vm.ToValue(nil)
	})
	objSetFn(o, "loadValid", func(call goja.FunctionCall) goja.Value {
		path, err := safeResolve(p.Root, "plugin", argString(call, 0))
		if err != nil {
			return vm.ToValue(nil)
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return vm.ToValue(nil)
		}
		if !isImageMagic(b) {
			return vm.ToValue(nil)
		}
		return vm.ToValue(base64.StdEncoding.EncodeToString(b))
	})
	objSetFn(o, "isImage", func(call goja.FunctionCall) goja.Value {
		b, err := base64.StdEncoding.DecodeString(argString(call, 0))
		if err != nil {
			return vm.ToValue(false)
		}
		return vm.ToValue(isImageMagic(b))
	})
	parent.Set("image", o)
}

// isImageMagic 校验字节头是否为常见图片格式。
func isImageMagic(b []byte) bool {
	if len(b) < 8 {
		return false
	}
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return true
	case bytes.HasPrefix(b, []byte("\xFF\xD8\xFF")):
		return true
	case bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a")):
		return true
	case bytes.HasPrefix(b, []byte("RIFF")) && len(b) > 12 && string(b[8:12]) == "WEBP":
		return true
	case bytes.HasPrefix(b, []byte("BM")):
		return true
	}
	return false
}

// queryMessages 查询最近记忆消息。
func queryMessages(limit int) ([]map[string]any, int, error) {
	if !file.IsMemoryInitialized() {
		return nil, 0, fmt.Errorf("记忆库未初始化")
	}
	entries, total := file.MemoryGetDocuments(defaultMemCollection, 0, limit)
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{
			"id":      e.ID,
			"role":    e.Role,
			"content": e.Content,
		})
	}
	return out, total, nil
}

// searchMessages 向量语义检索记忆库。
func searchMessages(query string, limit int) ([]map[string]any, int, error) {
	if !file.IsMemoryInitialized() {
		return nil, 0, fmt.Errorf("记忆库未初始化")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results, err := file.MemoryQueryMessagesWithContent(ctx, defaultMemCollection, query, limit)
	if err != nil {
		// 回退到顺序文档列表
		entries, total := file.MemoryGetDocuments(defaultMemCollection, 0, limit)
		out := make([]map[string]any, 0, len(entries))
		for _, e := range entries {
			out = append(out, map[string]any{"id": e.ID, "role": e.Role, "content": e.Content})
		}
		return out, total, nil
	}
	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]any{
			"id":         r.ID,
			"role":       r.Role,
			"content":    r.Content,
			"similarity": r.Similarity,
		})
	}
	return out, len(out), nil
}

// defaultMemCollection 默认记忆集合名（与 crystal_astral 保持一致）。
const defaultMemCollection = "lunar_messages"

// safeResolve 校验相对路径不越界，返回根目录内的绝对路径。
func safeResolve(root, scope, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("%s 路径不能为空", scope)
	}
	clean := filepath.Clean(rel)
	if clean == "." {
		return "", fmt.Errorf("%s 路径不能为目录", scope)
	}
	abs := filepath.Join(root, clean)
	rootAbs, _ := filepath.Abs(root)
	absAbs, _ := filepath.Abs(abs)
	if !strings.HasPrefix(absAbs, rootAbs+string(filepath.Separator)) && absAbs != rootAbs {
		return "", fmt.Errorf("%s 路径越界: %s", scope, rel)
	}
	return absAbs, nil
}
