package resource

// observer.go persistence 落盘观察者的实现 —— 三棵内容仓库树的句柄追踪钩子
//
// persistence 只认识「文件原语」与观察者接口；本包在 Init 成功后注册观察者，
// 因此写盘 / 移动 / 改名 / 删除 / 读取发生在 documents、audios、multimedia 下时，
// 句柄映射会自动跟随，不依赖调用方逐个记得登记。
//
// 约束：观察者内部**不得**再调用 persistence 的写盘原语（避免递归）；
// 登记失败只记 WARN，不回滚已完成的落盘结果。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
)

// observer 落盘观察者
type observer struct{}

// AfterWrite 写盘完成后登记句柄，返回最终保留路径与句柄
func (observer) AfterWrite(rel string) (canonicalRel, handle string) {
	canonical, resultHandle, err := RegisterPath(rel)
	if err != nil {
		logger.SubWarn("Persistence", "Track", "落盘登记句柄失败(%s): %v", rel, err)
	}
	if canonical == "" {
		canonical = normalizeRel(rel)
	}
	return canonical, resultHandle
}

// AfterMove 移动 / 改名完成后改写句柄映射，必要时为新位置补登记
func (observer) AfterMove(oldRel, newRel string) {
	oldRel, newRel = normalizeRel(oldRel), normalizeRel(newRel)
	if oldRel == newRel {
		return
	}
	// 目标落在派生缓存里：句柄体系不再覆盖该实体，移除旧映射
	if IsDerivedStoragePath(newRel) {
		RemoveHandlesForPath(oldRel)
		return
	}
	RewriteHandlesForMove(oldRel, newRel)
	if !IsStoragePath(newRel) {
		return
	}
	if _, err := UpsertPath(newRel); err != nil {
		logger.SubWarn("Persistence", "Track", "移动后登记句柄失败(%s): %v", newRel, err)
	}
}

// AfterDelete 删除完成后移除对应句柄（实体丢、句柄清）
func (observer) AfterDelete(rel string) {
	RemoveHandlesForPath(rel)
}

// AfterRead 读取命中后刷新句柄续期；实体缺失时清理失效映射
func (observer) AfterRead(rel string) {
	TouchHandlesForRead(rel)
}

// Lookup 按路径查询句柄映射（只读，供文件列表展示）
func (observer) Lookup(rel string) (persistence.FileHandle, bool) {
	res, ok := ForPath(rel)
	if !ok {
		return persistence.FileHandle{}, false
	}
	return persistence.FileHandle{Handle: res.Handle, Kind: res.Kind, ContentHash: res.ContentHash}, true
}
