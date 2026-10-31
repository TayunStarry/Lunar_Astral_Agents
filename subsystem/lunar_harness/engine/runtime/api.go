package runtime

// ==== engine.* 能力实现（Go 侧，接入项目组件） ====

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/memory"

	"github.com/dop251/goja"

	// SQLite 驱动
	_ "github.com/mattn/go-sqlite3"
)

// ==== 广播 signal.all/target → signal.subscribe ====

// engineBroadcast 把一条内容分发给指定运行时（或 __all__）的广播订阅器。
func engineBroadcast(target string, payload any) {
	for _, p := range snapshotRuntimes() {
		if target != "__all__" && target != p.ID {
			continue
		}
		p.mu.Lock()
		loaded := p.loaded
		subs := append([]*eventSub(nil), p.signalSubs...)
		p.mu.Unlock()
		if !loaded {
			continue
		}
		for _, cb := range subs {
			go p.loop.RunOnLoop(func(vm *goja.Runtime) {
				if f, ok := goja.AssertFunction(cb.handler); ok {
					arg := vm.ToValue(payload)
					f(goja.Undefined(), arg)
				}
			})
		}
	}
}

// ==== 事件发布 event.publish ====

// enginePublish 在私有运行时事件总线上发起事件：派发给其它运行时的订阅器。
// 异步分发（fire-and-forget）：publish 自身不阻塞；跳过发起运行时自身订阅器，避免回调内自锁。
func enginePublish(from *plugin, topic string, payload any) {
	for _, q := range snapshotRuntimes() {
		if q == from {
			continue
		}
		go q.fireEvent(topic, payload)
	}
}

// ==== 图片记忆 memory.searchImage/storeImage/randomImage（全局图像类型检索） ====

// memorySearchImage 基于查询文本在全部集合中检索图像类型记忆，返回 { url|base64, similarity } 列表。
func memorySearchImage(query string, limit int) (any, error) {
	if err := ensureMemory(); err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	if strings.TrimSpace(query) == "" {
		return rwResult{Success: false, Error: "query 必填"}, nil
	}
	// 图片记忆分散在表情包/截图/图片素材等集合，稀疏媒体必须全局检索
	results, err := memory.Query(context.Background(), query, memory.TypeImage, limit,
		memory.QueryOptions{ForceGlobal: true})
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		item := map[string]any{"similarity": r.Similarity}
		// 新记忆只存 URL；历史记忆仍可能只有内联 base64。
		if r.URL != "" {
			item["url"] = r.URL
		} else {
			item["base64"] = r.Base64
		}
		out = append(out, item)
	}
	return map[string]any{"success": true, "results": out}, nil
}

// memoryStoreImage 添加一张图片（base64）。标签由记忆库多模态模型生成，内容自动归组。
func memoryStoreImage(image string) (any, error) {
	if err := ensureMemory(); err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	if strings.TrimSpace(image) == "" {
		return rwResult{Success: false, Error: "image 必填"}, nil
	}
	id, err := memory.AddImage(context.Background(), image, "auto", "")
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	return map[string]any{"success": true, "id": id}, nil
}

// memoryRandomImage 基于查询文本随机返回一张图像类型记忆；query 为空时按通用语义检索。
func memoryRandomImage(query string) (any, error) {
	if err := ensureMemory(); err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	q := query
	if strings.TrimSpace(q) == "" {
		q = "general"
	}
	results, err := memory.Query(context.Background(), q, memory.TypeImage, 3,
		memory.QueryOptions{ForceGlobal: true})
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	if len(results) == 0 {
		return rwResult{Success: false, Error: "无匹配的表情包"}, nil
	}
	pick := results[rand.Intn(len(results))]
	if pick.URL != "" {
		return map[string]any{"success": true, "url": pick.URL}, nil
	}
	// 历史记忆兼容：仅当没有 URL 时回退内联 base64
	return map[string]any{"success": true, "base64": pick.Base64}, nil
}

// ==== 跨运行时调用 callFunction ====

// engineCall 调用目标运行时的导出函数。目标不存在或未导出 → 抛「xxx 运行时拒绝响应」。
func engineCall(id, fnName string, args []any) (any, error) {
	p := runtimeFor(id)
	if p == nil {
		return nil, fmt.Errorf("%s 运行时拒绝响应", id)
	}
	return p.callExport(fnName, args)
}

// ==== 前端智能体 agent.synergy（私有运行时不再连接前端包） ====

func engineAgent(id, text string) (any, error) {
	_ = text
	return nil, fmt.Errorf("%s 运行时拒绝响应（私有 js 运行时未接入前端智能体通道）", id)
}

// ==== 加解密桥见 api_encoding.go（encoding.lunarEncoder / encoding.lunarDecoder） ====

// ==== 文件 file.write/read/delete（路径限定插件数据目录） ====

// scopedPath 把插件给定的相对路径约束在插件数据目录内。
// 用 filepath.Rel 判定越界（前缀 HasPrefix 会放行 "data" 同前缀的兄弟目录，如 ../database/）；
// 绝对路径与含盘符/UNC 前缀的输入（Windows 下 Join 会拼出越界路径）直接拒绝。
func scopedPath(p *plugin, path string) (string, error) {
	if filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return "", fmt.Errorf("路径越界: %s", path)
	}
	clean := filepath.Clean(filepath.Join(p.DataDir, path))
	rel, rerr := filepath.Rel(p.DataDir, clean)
	if rerr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径越界: %s", path)
	}
	_ = os.MkdirAll(filepath.Dir(clean), 0755)
	return clean, nil
}

func fileWrite(p *plugin, path, data string) (any, error) {
	real, err := scopedPath(p, path)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	if werr := os.WriteFile(real, []byte(data), 0644); werr != nil {
		return rwResult{Success: false, Error: werr.Error()}, nil
	}
	return rwResult{Success: true}, nil
}

func fileRead(p *plugin, path string) (any, error) {
	real, err := scopedPath(p, path)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	b, rerr := os.ReadFile(real)
	if rerr != nil {
		return rwResult{Success: false, Error: rerr.Error()}, nil
	}
	return rwResult{Success: true, Text: string(b)}, nil
}

func fileDelete(p *plugin, path string) (any, error) {
	real, err := scopedPath(p, path)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	if derr := os.Remove(real); derr != nil {
		return rwResult{Success: false, Error: derr.Error()}, nil
	}
	return rwResult{Success: true}, nil
}

// ==== 记忆库 memory.store/search（接入项目记忆库，全局类型化检索） ====

var memoryOnce sync.Once
var memoryErr error

// ensureMemory 惰性初始化项目记忆库实例（启动时一次性加载全部集合）。
func ensureMemory() error {
	memoryOnce.Do(func() {
		// 若宿主尚未初始化全局记忆库，则自建实例（baseDir 沿用项目 MemoryDBDir），
		// 使 engine.memory 在引擎自足场景下也能工作，而不仅依赖宿主预先调用 memory.Init。
		if !memory.Ready() {
			memory.Init(config.MemoryDBDir)
		}
		memoryErr = memory.EnsureReady()
	})
	return memoryErr
}

func memoryStore(v map[string]any) (any, error) {
	if err := ensureMemory(); err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	// 图片记忆 —— base64 字段走多模态标签理解嵌入，内容自动归组
	if b64, hasB64 := v["base64"].(string); hasB64 && strings.TrimSpace(b64) != "" {
		id, err := memory.AddImage(context.Background(), b64, "auto", "")
		if err != nil {
			return rwResult{Success: false, Error: err.Error()}, nil
		}
		return map[string]any{"success": true, "id": id}, nil
	}
	content, _ := v["content"].(string)
	if content == "" {
		return rwResult{Success: false, Error: "content/base64 必填"}, nil
	}
	var tags []string
	if raw, ok := v["tags"].([]any); ok {
		for _, t := range raw {
			if s, ok := t.(string); ok && s != "" {
				tags = append(tags, s)
			}
		}
	}
	// 文本记忆：有预设标签则以标签文本嵌入，无标签回退为正文全文嵌入；内容自动归组
	id, err := memory.AddTextWithTags(context.Background(), "ltp9", content, tags, "")
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	return map[string]any{"success": true, "id": id}, nil
}

// memorySearch 按语义检索指定类型记忆（默认文本），返回 { success, results: [MemoryHit] }（Result 统一风格）；
// 命中按相似度降序，基础设施错误不再抛异常。
func memorySearch(v map[string]any) (any, error) {
	if err := ensureMemory(); err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	query, _ := v["query"].(string)
	limit := 5
	if n, ok := v["limit"].(int64); ok && n > 0 && n <= 50 {
		limit = int(n)
	}
	mediaType := memory.TypeText
	if rawType, ok := v["type"].(string); ok && strings.TrimSpace(rawType) != "" {
		normalized, err := memory.NormalizeType(rawType)
		if err != nil {
			return rwResult{Success: false, Error: err.Error()}, nil
		}
		mediaType = normalized
	}
	// 插件可选择 force_global=true 跳过快速路由（稀疏数据、需要确保不漏时）
	forceGlobal, _ := v["force_global"].(bool)
	results, err := memory.Query(context.Background(), query, mediaType, limit,
		memory.QueryOptions{ForceGlobal: forceGlobal})
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		item := map[string]any{"content": r.Content, "similarity": r.Similarity, "type": r.Type, "collection": r.Collection}
		if r.URL != "" {
			item["url"] = r.URL
		}
		out = append(out, item)
	}
	return map[string]any{"success": true, "results": out}, nil
}

// ==== 数据库 database.*（实现在 api_database.go） ====
