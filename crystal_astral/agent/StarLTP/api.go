package StarLTP

// ==== engine.* 能力实现（Go 侧，接入项目组件） ====

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	module "LunarSubsystem/FileManager/module"
	"LunarSubsystem/GeneralConfig"
	lunardecoder "LunarSubsystem/LunarDecoder"
	"github.com/dop251/goja"

	// SQLite 驱动
	_ "github.com/mattn/go-sqlite3"
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

// ==== 加解密 engine.encoder / engine.decoder（复用 lunar_decoder） ====

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

func scopedPath(p *plugin, path string) (string, error) {
	clean := filepath.Clean(filepath.Join(p.DataDir, path))
	if !strings.HasPrefix(clean, p.DataDir) {
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

// ==== 记忆库 engine.memory（接入项目 FileManager 记忆库） ====

// ltp9MemoryCollection LTP9 记忆库使用的集合名。
const ltp9MemoryCollection = "ltp9_memory"

var memoryOnce sync.Once
var memoryErr error

// ensureMemory 惰性初始化项目记忆库实例并确保 ltp9 集合存在。
func ensureMemory() error {
	memoryOnce.Do(func() {
		if !module.IsMemoryInitialized() {
			if err := module.MemoryInitInstance(); err != nil {
				memoryErr = err
				return
			}
		}
		if module.MemoryGetCollectionInfo(ltp9MemoryCollection) == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30_000_000_000)
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

func memorySearch(v map[string]any) (any, error) {
	if err := ensureMemory(); err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	query, _ := v["query"].(string)
	limit := 5
	if n, ok := v["limit"].(int64); ok && n > 0 && n < 50 {
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

// ==== 数据库 engine.database（接入 SQLite） ====

var (
	dbOnce sync.Once
	dbInst *sql.DB
	dbPath string
	dbErr  error
)

// ensureDB 惰性打开 LTP9 SQLite 数据库文件（local_data/database/ltp9.db，与项目数据根目录一致）。
func ensureDB() (*sql.DB, error) {
	dbOnce.Do(func() {
		execPath, err := os.Executable()
		if err != nil {
			dbErr = err
			return
		}
		dir := filepath.Join(filepath.Dir(execPath), *GeneralConfig.LocalDir, "database")
		_ = os.MkdirAll(dir, 0755)
		dbPath = filepath.Join(dir, "ltp9.db")
		dbInst, dbErr = sql.Open("sqlite3", dbPath)
		if dbErr != nil {
			return
		}
		_, dbErr = dbInst.Exec("PRAGMA journal_mode=WAL;")
	})
	if dbInst == nil && dbErr == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}
	return dbInst, dbErr
}

func databaseQuery(q string, params []any) (any, error) {
	db, err := ensureDB()
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	args := anySlice(params)
	rows, err := db.Query(q, args...)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if rows.Scan(ptrs...) != nil {
			continue
		}
		row := map[string]any{}
		for i, c := range cols {
			row[c] = normalizeVal(vals[i])
		}
		out = append(out, row)
	}
	return map[string]any{"success": true, "rows": out}, nil
}

func databaseExec(q string, params []any) (any, error) {
	db, err := ensureDB()
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	args := anySlice(params)
	res, err := db.Exec(q, args...)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	rows, _ := res.RowsAffected()
	return map[string]any{"success": true, "rows_affected": rows}, nil
}

func anySlice(in []any) []any {
	if in == nil {
		return []any{}
	}
	return in
}

// normalizeVal 把 sqlite 返回的类型规整为 JS 友好值（[]byte → string）。
func normalizeVal(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case int64:
		return float64(t)
	}
	return v
}
