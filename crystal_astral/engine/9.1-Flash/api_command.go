package StarLTP

// ==== command 指令系统（常驻基础能力） ====
// 插件以 command.register(name, pattern, handler, options) 注册「/xxx」类指令，
// 引擎侧经 Command(pluginID, text) / CommandAll(text, context) 触发：按指令名/别名
// 或首个匹配正则的处理器派发。处理器签名 (match, context) => result，
// match 为捕获组数组，context 为可选上下文。

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dop251/goja"
)

// bindCommand command.register（常驻注入）。
func bindCommand(vm *goja.Runtime, p *plugin) *goja.Object {
	c := vm.NewObject()
	c.Set("register", func(name, pattern string, handler goja.Value, options map[string]any) (any, error) {
		if name == "" {
			return rwResult{Success: false, Error: "指令名不能为空"}, nil
		}
		if handler == nil || !isFunc(handler) {
			return rwResult{Success: false, Error: "handler 需为函数"}, nil
		}
		pat := pattern
		// 兼容 /pattern/flags 分隔符写法
		if len(pat) > 1 && pat[0] == '/' {
			if idx := strings.IndexByte(pat[1:], '/'); idx >= 0 {
				pat = pat[1 : idx+1]
			}
		}
		// pattern 为空时仅作指令名/别名精确匹配，不参与正则回退
		var re *regexp.Regexp
		if pat != "" {
			var rerr error
			re, rerr = regexp.Compile(pat)
			if rerr != nil {
				return rwResult{Success: false, Error: "正则编译失败: " + rerr.Error()}, nil
			}
		}
		aliases := []string{}
		if m, ok := options["aliases"].([]any); ok {
			for _, a := range m {
				if s, ok := a.(string); ok && s != "" {
					aliases = append(aliases, s)
				}
			}
		}
		entry := &commandEntry{name: name, re: re, handler: handler, aliases: aliases}
		p.mu.Lock()
		p.commands[name] = entry
		for _, a := range aliases {
			p.commands[a] = entry
		}
		p.mu.Unlock()
		return rwResult{Success: true}, nil
	})
	return c
}

// Command 触发某插件指令。text 匹配注册时的指令名/别名或正则；
// context 为可选的用户/群等上下文。返回处理器结果；未命中返回「指令未匹配」。
func Command(pluginID, text string, context map[string]any) (any, error) {
	if Engine == nil || pluginID == "" {
		return nil, fmt.Errorf("指令未匹配")
	}
	p := Engine.pluginFor(pluginID)
	if p == nil || !p.loaded {
		return nil, fmt.Errorf("指令未匹配")
	}
	return p.dispatchCommand(text, context)
}

// CommandAll 向全部插件派发，返回 插件ID → 指令结果 映射（仅命中者）。
func CommandAll(text string, context map[string]any) map[string]any {
	if Engine == nil {
		return map[string]any{}
	}
	out := map[string]any{}
	for _, p := range Engine.snapshotPlugins() {
		if !p.loaded {
			continue
		}
		if res, err := p.dispatchCommand(text, context); err == nil {
			out[p.ID] = res
		}
	}
	return out
}

// dispatchCommand 在当前插件内按指令名/别名/正则匹配并调用处理器。
// 先精确匹配指令名与别名，再按注册正则逐一匹配首条命中；匹配到即调用 handler。
func (p *plugin) dispatchCommand(text string, context map[string]any) (any, error) {
	p.mu.Lock()
	byName := p.commands[text]
	p.mu.Unlock()
	if byName != nil {
		return p.callCommandHandler(byName, []string{text}, context)
	}
	p.mu.Lock()
	var found *commandEntry
	for _, e := range p.commands {
		if e.re != nil && e.re.MatchString(text) {
			found = e
			break
		}
	}
	p.mu.Unlock()
	if found == nil {
		return nil, fmt.Errorf("指令未匹配")
	}
	matches := found.re.FindStringSubmatch(text)
	return p.callCommandHandler(found, matches, context)
}

// callCommandHandler 在插件事件循环内调用指令处理器（match 数组 + context）。
func (p *plugin) callCommandHandler(e *commandEntry, match []string, context map[string]any) (any, error) {
	if e == nil || e.handler == nil {
		return nil, fmt.Errorf("指令未匹配")
	}
	return p.callFn(e.handler, match, context)
}
