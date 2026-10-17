package module

// archive_gc.go 内容仓库回收 —— 「回收只删 blob，保留目录行」
//
// 设计依据：docs/adr/0001「保留策略与降级语义」。
// 回收顺序：等级（ephemeral → soft → normal）→ 同等级内最久未用（used_at 升序）。
// 回收动作只删除物理 blob 并把 resources.state 置为 lost：
// 句柄 @I.007 / @D.003 仍可解析，取用时降级为描述（图片）或大纲（文档）。
//
// 「正常等级」条目仅在总容量超限时才被回收；只要未超限，用户资料永不被删。

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"os"
	"path/filepath"
)

// GCResult 一次回收的结果
type GCResult struct {
	// Scanned 参与评估的资源条数
	Scanned int `json:"scanned"`
	// Collected 被回收（blob 删除）的条数
	Collected int `json:"collected"`
	// FreedBytes 释放的字节数
	FreedBytes int64 `json:"freed_bytes"`
	// RemainingBytes 回收后目录记录的总体积
	RemainingBytes int64 `json:"remaining_bytes"`
	// OverQuota 回收后仍超配额（说明受保护条目占满）
	OverQuota bool `json:"over_quota"`
}

// quotaBytes 内容仓库容量上限（字节）
func quotaBytes() int64 {
	mb := *GeneralConfig.ArchiveQuotaMB
	if mb <= 0 {
		mb = 2048
	}
	return int64(mb) * 1024 * 1024
}

// maxBlobBytes 单资源入仓体积上限（字节）
func maxBlobBytes() int64 {
	mb := *GeneralConfig.ArchiveMaxBlobMB
	if mb <= 0 {
		mb = 20
	}
	return int64(mb) * 1024 * 1024
}

// ArchiveCollect 执行一次容量回收，返回回收统计
//
// force 为 true 时不检查配额，直接按等级顺序回收（运维手动触发用）。
func ArchiveCollect(force bool) (GCResult, error) {
	result := GCResult{}
	if err := EnsureArchiveInitialized(); err != nil {
		return result, err
	}
	quota := quotaBytes()
	total, err := ArchiveDatabase.totalActiveBytes()
	if err != nil {
		return result, err
	}
	result.RemainingBytes = total
	if !force && total <= quota {
		return result, nil
	}

	// 回收顺序：ephemeral → soft → normal，各级内按 used_at 升序（最久未用优先）
	for _, retain := range []string{RetainEphemeral, RetainSoft, RetainNormal} {
		candidates, err := ArchiveDatabase.collectCandidates(retain, 500)
		if err != nil {
			return result, err
		}
		for _, res := range candidates {
			result.Scanned++
			// 只处理「归档私有副本」：前端/引擎写入的实体（blob_owned=false）归创建者所有，
			// 回收器无权删除，否则会连带毁掉用户可见的图片与文档
			if !res.BlobOwned {
				continue
			}
			// 仅当没有其它目录行引用同一实体时才删除
			shared, err := ArchiveDatabase.blobReferencedByOthers(res.BlobPath, res.ID)
			if err != nil {
				continue
			}
			// 第二道守卫：同一实体若被任何「非归档所有」的目录行引用，说明它同时是前端/引擎的
			// 可见文件；内容寻址下同内容共用实体，归属必须按最保守的一侧处理，一律不删
			if !shared {
				if unowned, refErr := ArchiveDatabase.blobReferencedByUnowned(res.BlobPath, res.ID); refErr == nil && unowned {
					shared = true
				}
			}
			if !shared {
				abs := filepath.Join(filepath.Clean(*GeneralConfig.LocalDir), filepath.FromSlash(res.BlobPath))
				if info, statErr := os.Stat(abs); statErr == nil {
					if rmErr := os.Remove(abs); rmErr != nil {
						LoggerGeneral.Warn("FileManager", "回收 blob 失败(%s): %v", res.Handle, rmErr)
						continue
					}
					result.FreedBytes += info.Size()
					result.RemainingBytes -= info.Size()
				}
			}
			// blob 被其它目录行共享时，该行语义上仍然可用，不做降级标记
			// （句柄指向同一份内容，删了别人的实体不等于自己取不到）
			// 只有实体确已不存在才标记 lost
			abs := filepath.Join(filepath.Clean(*GeneralConfig.LocalDir), filepath.FromSlash(res.BlobPath))
			if !fileExists(abs) {
				if err := ArchiveDatabase.MarkState(res.ID, StateLost); err != nil {
					LoggerGeneral.Warn("FileManager", "标记资源丢失状态失败(%s): %v", res.Handle, err)
					continue
				}
			}
			result.Collected++
			if !force && result.RemainingBytes <= quota {
				return result, nil
			}
		}
		// 配额模式下已回到阈值内即停止；force 模式按等级继续处理下一级
		if !force && result.RemainingBytes <= quota {
			return result, nil
		}
	}
	result.OverQuota = result.RemainingBytes > quota
	if result.Collected > 0 {
		LoggerGeneral.Info("FileManager", "资源仓库回收完成：回收 %d 项，释放 %d 字节，剩余 %d 字节",
			result.Collected, result.FreedBytes, result.RemainingBytes)
	}
	return result, nil
}
