package YaraLTP

// ==== 单插件 goja 沙箱：加载 / 卸载 / 串行执行 ====

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LoggerGeneral"
	"github.com/dop251/goja"
)

// newPlugin 依据包目录构造插件实例（不加载 VM）。
func newPlugin(dirName, root, id, title string) *plugin {
	p := &plugin{
		ID:         id,
		DirName:    dirName,
		Title:      title,
		Root:       root,
		MainPath:   filepath.Join(root, DefaultMain),
		ConfigPath: filepath.Join(root, DefaultConfigFile),
		DataDir:    filepath.Join(root, DataDirName),

		hooks:        map[string][]*hookSub{},
		events:       map[string][]*eventSub{},
		commands:     map[string]*commandDef{},
		tools:        map[string]*toolDef{},
		apis:         map[string]*apiDef{},
		llmProviders: map[string]*llmProvider{},
	}
	if err := os.MkdirAll(p.DataDir, 0755); err != nil {
		LoggerGeneral.Warn(ServiceName, "插件 %s data 目录创建失败: %v", id, err)
	}
	return p
}

// readMeta 从包目录读取 metadata.json 的 LTP3 识别信息。非 LTP3 返回 false。
func readMeta(root string) (id, title string, tags []string, isLTP3 bool, err error) {
	raw, rerr := os.ReadFile(filepath.Join(root, "metadata.json"))
	if rerr != nil {
		return "", "", nil, false, rerr
	}
	var meta struct {
		ID    string   `json:"id"`
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
	}
	if jerr := json.Unmarshal(raw, &meta); jerr != nil {
		return "", "", nil, false, jerr
	}
	for _, t := range meta.Tags {
		if strings.EqualFold(t, LTP3Tag) {
			return meta.ID, meta.Title, meta.Tags, true, nil
		}
	}
	return meta.ID, meta.Title, meta.Tags, false, nil
}

// load 创建 goja 沙箱、绑定 yara.*、执行 index.js、调用 onLoad。
func (p *plugin) load() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.loaded {
		return nil
	}

	// 1. 读取配置（config.yaml 只读解析；失败仅告警，不阻断加载）
	if raw, err := os.ReadFile(p.ConfigPath); err == nil {
		if cfg, perr := parseYAML(string(raw)); perr == nil {
			p.config = cfg
		} else {
			LoggerGeneral.Warn(ServiceName, "插件 %s config.yaml 解析失败: %v", p.ID, perr)
		}
	}

	// 2. 读取主逻辑源码
	code, err := os.ReadFile(p.MainPath)
	if err != nil {
		p.loadErr = fmt.Sprintf("读取 index.js 失败: %v", err)
		return fmt.Errorf("%s", p.loadErr)
	}

	// 3. 创建独立 goja 虚拟机
	vm := goja.New()
	vm.SetFieldNameMapper(goja.UncapFieldNameMapper())
	p.vm = vm

	// 4. 绑定 yara 全局 API
	bindYara(p)

	// 5. 注册全局常量
	vm.Set("YaraEvents", YaraEvents)
	vm.Set("YaraHooks", YaraHooks)

	// 6. 执行主脚本（顶层注册各订阅/指令/工具）
	if _, rerr := vm.RunString(string(code)); rerr != nil {
		p.loadErr = fmt.Sprintf("index.js 执行失败: %v", rerr)
		p.vm = nil
		return fmt.Errorf("%s", p.loadErr)
	}

	// 7. 捕获生命周期函数（可选）
	if v := vm.Get("onLoad"); isJSFunc(v) {
		p.onLoadFn = v
	}
	if v := vm.Get("onUnload"); isJSFunc(v) {
		p.onUnloadFn = v
	}
	if v := vm.Get("onConfigUpdate"); isJSFunc(v) {
		p.onConfigUpdateFn = v
	}

	// 8. 调用 onLoad
	if p.onLoadFn != nil {
		if _, rerr := p.callFn(p.onLoadFn); rerr != nil {
			LoggerGeneral.Warn(ServiceName, "插件 %s onLoad 执行异常: %v", p.ID, rerr)
		}
	}

	p.loaded = true
	p.loadErr = ""
	// 广播生命周期：插件加载完成
	emitBus(lifecycleMessage{Type: "ltp3/lifecycle", Event: "ON_START", Plugin: p.ID, Title: p.Title})
	LoggerGeneral.Info(ServiceName, "LTP3 插件已加载: %s (%s)", p.ID, p.Root)
	return nil
}

// unload 调用 onUnload 并释放 goja 虚拟机。
func (p *plugin) unload() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.onUnloadFn != nil && p.vm != nil {
		if _, rerr := p.callFn(p.onUnloadFn); rerr != nil {
			LoggerGeneral.Warn(ServiceName, "插件 %s onUnload 执行异常: %v", p.ID, rerr)
		}
	}
	p.vm = nil
	p.loaded = false
	// 清空注册表
	p.hooks = map[string][]*hookSub{}
	p.events = map[string][]*eventSub{}
	p.commands = map[string]*commandDef{}
	p.tools = map[string]*toolDef{}
	p.apis = map[string]*apiDef{}
	p.llmProviders = map[string]*llmProvider{}
	p.toolRegOrder = nil
	p.config = nil
	p.onLoadFn, p.onUnloadFn, p.onConfigUpdateFn = nil, nil, nil
	emitBus(lifecycleMessage{Type: "ltp3/lifecycle", Event: "ON_STOP", Plugin: p.ID, Title: p.Title})
	LoggerGeneral.Info(ServiceName, "LTP3 插件已卸载: %s", p.ID)
}

// callFn 在插件 VM 中调用一个 JS 函数并导出返回值。
func (p *plugin) callFn(fn goja.Value, args ...any) (any, error) {
	if p.vm == nil {
		return nil, fmt.Errorf("插件 %s 虚拟机不存在", p.ID)
	}
	f, ok := goja.AssertFunction(fn)
	if !ok {
		return nil, fmt.Errorf("非函数对象")
	}
	callArgs := make([]goja.Value, 0, len(args))
	for _, a := range args {
		callArgs = append(callArgs, p.vm.ToValue(a))
	}
	v, err := f(goja.Undefined(), callArgs...)
	if err != nil {
		// 把 goja 异常包装为可读错误
		if jex, ok := err.(*goja.Exception); ok {
			return nil, fmt.Errorf("%v", jex.Value())
		}
		return nil, err
	}
	return v.Export(), nil
}