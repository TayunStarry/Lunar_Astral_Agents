package StarLTP

// ==== engine.* 能力实现（Go 侧，接入项目组件） ====

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	module "LunarSubsystem/FileManager/module"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"

	"github.com/dop251/goja"

	// SQLite 驱动
	_ "github.com/mattn/go-sqlite3"
)

// ==== 广播 signal.all/target → signal.subscribe ====

func engineBroadcast(target string, payload any) {
	if Engine == nil {
		return
	}
	Engine.mu.RLock()
	outbound := Engine.outbound
	Engine.mu.RUnlock()
	if outbound != nil {
		outbound(target, payload)
	}
	for _, p := range Engine.snapshotPlugins() {
		if target != "__all__" && target != p.ID {
			continue
		}
		p.mu.Lock()
		cbs := append([]*eventSub(nil), p.signalSubs...)
		p.mu.Unlock()
		for _, cb := range cbs {
			go p.loop.RunOnLoop(func(vm *goja.Runtime) {
				if f, ok := goja.AssertFunction(cb.handler); ok {
					arg := vm.ToValue(payload)
					f(goja.Undefined(), arg)
				}
			})
		}
	}
}

// ==== 事件发布 engine.event.publish ====

// publishDispatchSem 同时派发中的 publish 事件上限：插件互激 re-publish（A→B→A…）时
// 兜底防止 goroutine 与 CPU 无限爆炸；满载后丢弃本次派发并告警。
var publishDispatchSem = make(chan struct{}, 256)

// enginePublish 插件在事件总线上主动发起事件：派发给其它插件的订阅器，并转发给宿主 outbound 供外部客户端消费。
// 异步分发（fire-and-forget）：publish 自身不阻塞；跳过发起插件自身订阅器，避免回调内自锁/自循环。
func enginePublish(from *plugin, topic string, payload any) {
	if Engine == nil {
		return
	}
	// 转发给宿主 outbound（外部客户端经 /ws 订阅消费）
	Engine.mu.RLock()
	outbound := Engine.outbound
	Engine.mu.RUnlock()
	if outbound != nil {
		outbound(topic, payload)
	}
	// 派发给其它插件的订阅器（在各自 loop 内执行，不占用发起插件的 loop）
	for _, q := range Engine.snapshotPlugins() {
		if q == from {
			continue
		}
		select {
		case publishDispatchSem <- struct{}{}:
		default:
			LoggerGeneral.Warn(ServiceName, "事件发布派发并发已达上限，丢弃 topic=%s 发往 %s 的一次派发", topic, q.ID)
			continue
		}
		go func(q *plugin) {
			defer func() { <-publishDispatchSem }()
			q.fireEvent(topic, payload)
		}(q)
	}
}

// ==== 图片记忆 memory.searchImage/storeImage/randomImage（allow-memory，复用项目记忆库 stickers 集合） ====

// stickerColOnce / stickerColErr 惰性确保表情包集合（image 型）就绪。
var (
	stickerColOnce sync.Once
	stickerColErr  error
)

// ensureStickerCollection 确保项目记忆库与 stickers 集合（image 型）可访问。
func ensureStickerCollection() error {
	stickerColOnce.Do(func() {
		if err := ensureMemory(); err != nil {
			stickerColErr = err
			return
		}
		if module.MemoryGetCollectionInfo(StickerCollection) == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			stickerColErr = module.CollectionInit(ctx, StickerCollection, *GeneralConfig.SearchEmbeddingModel, module.CollectionTypeImage)
		}
	})
	return stickerColErr
}

// memorySearchImage 基于查询文本从 stickers 集合检索图片，返回 { image, similarity } 列表。
func memorySearchImage(query string, limit int) (any, error) {
	if err := ensureStickerCollection(); err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	if strings.TrimSpace(query) == "" {
		return rwResult{Success: false, Error: "query 必填"}, nil
	}
	results, err := module.MemoryQueryMessagesWithContent(context.Background(), StickerCollection, query, limit)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]any{"image": r.Image, "similarity": r.Similarity})
	}
	return map[string]any{"success": true, "results": out}, nil
}

// memoryStoreImage 往 stickers 集合添加一张图片（base64）。标签由记忆库 LLM 自动生成。
func memoryStoreImage(image string) (any, error) {
	if err := ensureStickerCollection(); err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	if strings.TrimSpace(image) == "" {
		return rwResult{Success: false, Error: "image 必填"}, nil
	}
	id, err := module.MemoryAddImage(context.Background(), StickerCollection, image, "auto", "")
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	return map[string]any{"success": true, "id": id}, nil
}

// memoryRandomImage 基于查询文本随机返回一张图片；query 为空时按通用语义检索。
func memoryRandomImage(query string) (any, error) {
	if err := ensureStickerCollection(); err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	q := query
	if strings.TrimSpace(q) == "" {
		q = "general"
	}
	results, err := module.MemoryQueryMessagesWithContent(context.Background(), StickerCollection, q, 3)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	if len(results) == 0 {
		return rwResult{Success: false, Error: "无匹配的表情包"}, nil
	}
	pick := results[rand.Intn(len(results))]
	return map[string]any{"success": true, "image": pick.Image}, nil
}

// ==== 跨包调用 callFunction ====

// engineCall 调用目标插件导出的函数。目标插件不存在或未导出 → 抛「xxx 包拒绝响应」。
func engineCall(id, fnName string, args []any) (any, error) {
	if Engine == nil {
		return nil, fmt.Errorf("%s 包拒绝响应", id)
	}
	p := Engine.pluginFor(id)
	if p == nil {
		return nil, fmt.Errorf("%s 包拒绝响应", id)
	}
	return p.callExport(fnName, args)
}

// ==== 前端智能体 agent.synergy（宿主注入的真实通道） ====

func engineAgent(id, text string) (any, error) {
	outboundMu.RLock()
	inv := agentInvoker
	outboundMu.RUnlock()
	if inv == nil {
		return nil, fmt.Errorf("%s 包拒绝响应", id)
	}
	return inv(id, text)
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

// ==== 记忆库 memory.store/search（接入项目 FileManager 记忆库） ====

// ltp9MemoryCollection LTP9 记忆库使用的集合名。
const ltp9MemoryCollection = "ltp9_memory"

var memoryOnce sync.Once
var memoryErr error

// ensureMemory 惰性初始化项目记忆库实例并确保 ltp9 集合存在。
func ensureMemory() error {
	memoryOnce.Do(func() {
		// 若宿主尚未初始化全局记忆库，则自建实例（baseDir 沿用项目 MemoryDBDir），
		// 使 engine.memory 在引擎自足场景下也能工作，而不仅依赖宿主预先 InitMemoryDB。
		if !module.IsMemoryInitialized() {
			module.InitMemoryDB(*GeneralConfig.MemoryDBDir)
			if err := module.MemoryInitInstance(); err != nil {
				memoryErr = err
				return
			}
		}
		if module.MemoryGetCollectionInfo(ltp9MemoryCollection) == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := module.CollectionInit(ctx, ltp9MemoryCollection, *GeneralConfig.SearchEmbeddingModel, module.CollectionTypeText); err != nil {
				memoryErr = err
			}
		}
	})
	return memoryErr
}

func memoryStore(v map[string]any) (any, error) {
	if err := ensureMemory(); err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	content, _ := v["content"].(string)
	if content == "" {
		return rwResult{Success: false, Error: "content 必填"}, nil
	}
	var tags []string
	if raw, ok := v["tags"].([]any); ok {
		for _, t := range raw {
			if s, ok := t.(string); ok && s != "" {
				tags = append(tags, s)
			}
		}
	}
	id, err := module.MemoryAddMessageWithTags(context.Background(), ltp9MemoryCollection, "ltp9", content, tags)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	return map[string]any{"success": true, "id": id}, nil
}

// memorySearch 按语义检索记忆，返回 { success, results: [MemoryHit] }（Result 统一风格）；
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
	results, err := module.MemoryQueryMessagesWithContent(context.Background(), ltp9MemoryCollection, query, limit)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]any{"content": r.Content, "similarity": r.Similarity})
	}
	return map[string]any{"success": true, "results": out}, nil
}

// ==== 数据库 database.*（allow-database，实现在 api_database.go） ====
