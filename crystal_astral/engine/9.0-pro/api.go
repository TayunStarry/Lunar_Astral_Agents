package ltp9

// ==== engine.* 能力实现（Go 侧，桥接琉璃基板 / 宿主注入） ====

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"

	file "LunarSubsystem/FileManager/module"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	lunardecoder "LunarSubsystem/LunarDecoder"
	"github.com/dop251/goja"
)

// ==== 广播 engine.signal / frontEvent.signal ====

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
		cbs := append([]*eventSub(nil), p.frontSignal...)
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

// publishDispatchSem 同时派发中的 publish 事件上限。
var publishDispatchSem = make(chan struct{}, 256)

// enginePublish 插件在事件总线上主动发起事件：派发给其它插件的订阅器，并转发给宿主 outbound。
func enginePublish(from *plugin, topic string, payload any) {
	if Engine == nil {
		return
	}
	Engine.mu.RLock()
	outbound := Engine.outbound
	Engine.mu.RUnlock()
	if outbound != nil {
		outbound(topic, payload)
	}
	for _, q := range Engine.snapshotPlugins() {
		if q == from {
			continue
		}
		select {
		case publishDispatchSem <- struct{}{}:
		default:
			LoggerGeneral.Warn(ServiceName, "事件发布派发并发已达上限，丢弃一次派发 topic=%s target=%s", topic, q.ID)
			continue
		}
		go func(q *plugin) {
			defer func() { <-publishDispatchSem }()
			q.fireEvent(topic, payload)
		}(q)
	}
}

// ==== 表情包 engine.emoji（allow-memory，桥接琉璃记忆库 stickers 集合） ====

// stickerOnce / stickerErr 惰性确保 stickers 集合就绪。
var (
	stickerOnce sync.Once
	stickerErr  error
)

// ensureStickers 确保琉璃记忆库与 stickers 集合（image 型）可访问。
func ensureStickers() {
	stickerOnce.Do(func() {
		if !file.IsMemoryInitialized() {
			stickerErr = fmt.Errorf("记忆库未初始化")
			return
		}
		if file.MemoryGetCollectionInfo(StickerCollection) == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30_000_000_000)
			defer cancel()
			stickerErr = file.CollectionInit(ctx, StickerCollection, *GeneralConfig.AgentEmbeddingModel)
		}
	})
}

// emojiQuery 从 stickers 集合按语义检索表情，返回 {image, similarity} 列表；
// random=true 时从结果随机返回单张。
func emojiQuery(query string, limit int, random bool) (any, error) {
	ensureStickers()
	if stickerErr != nil {
		return rwResult{Success: false, Error: "未接入表情包(stickers 集合，宿主未注入记忆库): " + stickerErr.Error()}, nil
	}
	results, err := file.MemoryQueryMessagesWithContent(context.Background(), StickerCollection, query, limit)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	if len(results) == 0 {
		if random {
			return rwResult{Success: false, Error: "未匹配到表情"}, nil
		}
		return map[string]any{"success": true, "results": []any{}}, nil
	}
	if random {
		pick := results[rand.Intn(len(results))]
		return map[string]any{"success": true, "image": pick.Base64}, nil
	}
	out := make([]any, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]any{"image": r.Base64, "similarity": r.Similarity})
	}
	return map[string]any{"success": true, "results": out}, nil
}

func engineEmojiSearch(query string, limit int) (any, error) {
	if query == "" {
		query = "general"
	}
	return emojiQuery(query, limit, false)
}

func engineEmojiRandom(query string) (any, error) {
	return emojiQuery(query, 3, true)
}

func engineEmojiStore(image string) (any, error) {
	return rwResult{Success: false, Error: "表情包存储未接入(仅支持查询/随机)"}, nil
}

// ==== 跨包调用 engine.call ====

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

// ==== 前端智能体 engine.agent（注入的真实通道） ====

func engineAgent(id, text string) (any, error) {
	outboundMu.RLock()
	inv := agentInvoker
	outboundMu.RUnlock()
	if inv == nil {
		return nil, fmt.Errorf("%s 包拒绝响应", id)
	}
	return inv(id, text)
}

// ==== 加解密 engine.encoder / engine.decoder（复用琉璃 LunarDecoder） ====

func engineEncode(key, content string) (string, error) {
	outs, err := lunardecoder.EncodeFilesWithKeyString([]lunardecoder.FileData{{Name: "m", Data: []byte(content)}}, key)
	if err != nil {
		return "", err
	}
	return string(outs[0].Data), nil
}

func engineDecode(key, cipher string) (string, error) {
	outs, err := lunardecoder.DecodeFilesWithKeyString([]lunardecoder.FileData{{Name: "m", Data: []byte(cipher)}}, key)
	if err != nil {
		return "", err
	}
	if len(outs) == 0 {
		return "", fmt.Errorf("解码结果为空")
	}
	return string(outs[0].Data), nil
}

// ==== 文件 engine.file（路径限定插件数据目录） ====

// scopedPath 把插件给定的相对路径约束在插件数据目录内。
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

// ==== 记忆库 engine.memory（allow-memory，桥接琉璃 FileManager/module） ====
// 提供集合级 store/search 的轻量映射：search 走语义检索，store 走带向量写入。
// 若宿主记忆库未初始化，返回「未接入」占位。

const ltp9MemoryCollection = "ltp9_memory"

func memoryStore(v map[string]any) (any, error) {
	ensureStickers()
	if stickerErr != nil {
		return rwResult{Success: false, Error: "未接入记忆库(host 未注入): " + stickerErr.Error()}, nil
	}
	content, _ := v["content"].(string)
	if content == "" {
		return rwResult{Success: false, Error: "memory.store 需提供 content 字段"}, nil
	}
	// 集合未就绪时先初始化
	if file.MemoryGetCollectionInfo(ltp9MemoryCollection) == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30_000_000_000)
		defer cancel()
		if cerr := file.CollectionInit(ctx, ltp9MemoryCollection, *GeneralConfig.AgentEmbeddingModel); cerr != nil {
			return rwResult{Success: false, Error: "记忆库集合初始化失败: " + cerr.Error()}, nil
		}
	}
	if _, err := file.MemoryAddMessageSilent(context.Background(), ltp9MemoryCollection, "user", content); err != nil {
		return rwResult{Success: false, Error: "记忆写入失败: " + err.Error()}, nil
	}
	return rwResult{Success: true}, nil
}

func memorySearch(v map[string]any) (any, error) {
	ensureStickers()
	if stickerErr != nil {
		return rwResult{Success: false, Error: "未接入记忆库(host 未注入): " + stickerErr.Error()}, nil
	}
	q, _ := v["query"].(string)
	if q == "" {
		q, _ = v["content"].(string)
	}
	if q == "" {
		return rwResult{Success: false, Error: "memory.search 需提供 query/content 字段"}, nil
	}
	limit := 5
	if n, ok := v["limit"].(int64); ok && n > 0 {
		limit = int(n)
	} else if f, ok := v["limit"].(float64); ok && f > 0 {
		limit = int(f)
	}
	collection := ltp9MemoryCollection
	if c, ok := v["collection"].(string); ok && c != "" {
		collection = c
	}
	results, err := file.MemoryQueryMessagesWithContent(context.Background(), collection, q, limit)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	out := make([]any, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]any{"content": r.Content, "similarity": r.Similarity})
	}
	return map[string]any{"success": true, "results": out}, nil
}

// ==== 数据库 engine.database（allow-database） ====
// 原实现接入 SQLite，本移植未接入该外围，保留签名与门控，统一返回「未接入」占位。

func databaseQuery(q string, params []any) (any, error) {
	return rwResult{Success: false, Error: "未接入数据库(host 未注入)"}, nil
}

func databaseExec(q string, params []any) (any, error) {
	return rwResult{Success: false, Error: "未接入数据库(host 未注入)"}, nil
}