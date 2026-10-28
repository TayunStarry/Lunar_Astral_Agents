package persistence

// observer.go 落盘观察者：把底层文件原语的动作转告资源句柄系统
//
// 依赖方向 persistence ← engine/resource：本包只定义接口与转发函数，
// resource 在 Init 成功后注册 observer 实现。
// 未注册时（单元测试、极早期启动）全部退化为空操作，不影响文件操作本身。

import (
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
)

// SetResourceObserver 注册资源句柄观察者（由 engine/resource 调用）
func SetResourceObserver(obs ResourceObserver) {
	resourceObserver = obs
}

// NotifyWrite 写盘完成后通知句柄系统；返回最终保留路径与句柄
//
// canonicalRel 为空表示未登记（路径不在三棵内容仓库树内或登记失败），
// 调用方应继续使用自己写入的路径。
func NotifyWrite(rel string) (canonicalRel, handle string) {
	rel = normalizeRel(rel)
	if resourceObserver == nil {
		return rel, ""
	}
	return resourceObserver.AfterWrite(rel)
}

// NotifyMove 移动 / 改名完成后通知句柄系统
func NotifyMove(oldRel, newRel string) {
	oldRel, newRel = normalizeRel(oldRel), normalizeRel(newRel)
	if resourceObserver == nil || oldRel == "" || newRel == "" || oldRel == newRel {
		return
	}
	resourceObserver.AfterMove(oldRel, newRel)
}

// NotifyDelete 删除完成后通知句柄系统（实体丢、句柄清）
func NotifyDelete(rel string) {
	rel = normalizeRel(rel)
	if resourceObserver == nil || rel == "" {
		return
	}
	resourceObserver.AfterDelete(rel)
}

// NotifyRead 读取命中后通知句柄系统（续期 / 清理失效映射）
func NotifyRead(rel string) {
	rel = normalizeRel(rel)
	if resourceObserver == nil || rel == "" {
		return
	}
	resourceObserver.AfterRead(rel)
}

// LookupHandle 按路径查询句柄映射（只读，供文件列表展示）
func LookupHandle(rel string) (FileHandle, bool) {
	if resourceObserver == nil {
		return FileHandle{}, false
	}
	return resourceObserver.Lookup(normalizeRel(rel))
}

// normalizeRel 规整相对 LocalDir 的路径为斜杠形式
func normalizeRel(rel string) string {
	rel = strings.ReplaceAll(strings.TrimSpace(rel), "\\", "/")
	rel = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(rel)), "/")
	if rel == "." {
		return ""
	}
	return rel
}

// relativeToLocalDir 求绝对路径相对 LocalDir 的斜杠形式路径（不在其下时返回 false）
func relativeToLocalDir(abs string) (string, bool) {
	localDir := filepath.Clean(config.LocalDir)
	rel, err := filepath.Rel(localDir, abs)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}
