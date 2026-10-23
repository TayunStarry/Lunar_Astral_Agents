package persistence

// archive_vision.go 视觉读取副本缓存（media.db 内的派生数据）
//
// /file/miniature 端点把静态图变换为「约束尺寸 + 句柄叠印」的副本供思维链消费，
// 副本本体落在 multimedia/vision/（内容寻址、天然不可变），本表只做簿记：
// 记录内容哈希 → 副本路径的映射，命中即可跳过变换直接读盘。
// 容量超限时按 used_at 做 LRU 淘汰（连簿记行与副本文件一起清理；
// 副本是纯派生数据，被淘汰后下次读取按原图重建，不影响正确性）。
//
// 表在 media.db（媒体派生库），不再占用资源名录库 archive.db。

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// VisionCacheEntry 视觉副本缓存条目
type VisionCacheEntry struct {
	// 源图内容哈希（sha16）
	ContentHash string
	// 副本最大边长（像素）
	Side int
	// 叠印的句柄文本（@I.003）
	Tag string
	// 副本相对 LocalDir 路径
	CachePath string
	// 副本字节数
	Bytes int64
}

// visionCacheHandle 打开媒体派生库；失败时返回错误，由调用方降级为「无缓存」。
func visionCacheHandle() (*sql.DB, error) {
	db, err := mediaDatabase.open()
	if err != nil {
		return nil, fmt.Errorf("打开媒体派生库失败: %w", err)
	}
	return db, nil
}

// VisionCacheGet 按内容哈希与边长查缓存条目（未命中返回 false）
func VisionCacheGet(contentHash string, side int) (VisionCacheEntry, bool) {
	db, err := visionCacheHandle()
	if err != nil {
		return VisionCacheEntry{}, false
	}
	row := db.QueryRow(`SELECT tag, cache_path, bytes FROM vision_cache WHERE content_hash = ? AND side = ?`, contentHash, side)
	var entry VisionCacheEntry
	entry.ContentHash, entry.Side = contentHash, side
	if err := row.Scan(&entry.Tag, &entry.CachePath, &entry.Bytes); err != nil {
		return VisionCacheEntry{}, false
	}
	return entry, true
}

// VisionCachePut 写入/覆盖缓存条目（内容寻址，同键覆盖；used_at 同步续期）
func VisionCachePut(entry VisionCacheEntry) error {
	db, err := visionCacheHandle()
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	_, err = db.Exec(
		`INSERT INTO vision_cache (content_hash, side, tag, cache_path, bytes, created_at, used_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(content_hash, side) DO UPDATE SET
		 tag = excluded.tag, cache_path = excluded.cache_path, bytes = excluded.bytes,
		 created_at = excluded.created_at, used_at = excluded.used_at`,
		entry.ContentHash, entry.Side, entry.Tag, entry.CachePath, entry.Bytes, now, now,
	)
	if err != nil {
		return fmt.Errorf("写入视觉副本缓存失败: %w", err)
	}
	return nil
}

// VisionCacheTouch 缓存命中时续期（驱动 LRU：最近读取的副本最后被淘汰）
func VisionCacheTouch(contentHash string, side int) {
	db, err := visionCacheHandle()
	if err != nil {
		return
	}
	_, _ = db.Exec(`UPDATE vision_cache SET used_at = ? WHERE content_hash = ? AND side = ?`, time.Now().Unix(), contentHash, side)
}

// VisionCacheEvict 容量超限时按 LRU 淘汰视觉副本（连簿记行与磁盘文件一起清理）
//
// 淘汰顺序：used_at 最旧优先。被正在读取的副本文件在 Windows 上因句柄占用
// 删除会失败，此时跳过该条保留行，待下次淘汰再试。
//
// @param maxBytes 缓存池容量上限（字节，≤0 表示不限）
// @return 淘汰条数、释放字节数
func VisionCacheEvict(maxBytes int64) (int, int64) {
	if maxBytes <= 0 {
		return 0, 0
	}
	db, err := visionCacheHandle()
	if err != nil {
		return 0, 0
	}
	removed, freed := 0, int64(0)
	for {
		var total int64
		_ = db.QueryRow(`SELECT COALESCE(SUM(bytes), 0) FROM vision_cache`).Scan(&total)
		if total <= maxBytes {
			return removed, freed
		}
		// 取最久未用的一条
		row := db.QueryRow(`SELECT content_hash, side, cache_path, bytes FROM vision_cache ORDER BY used_at ASC, created_at ASC LIMIT 1`)
		var (
			contentHash, cachePath string
			side                   int
			bytes                  int64
		)
		if err := row.Scan(&contentHash, &side, &cachePath, &bytes); err != nil {
			// 表空但总量超限（簿记缺失）：直接返回，孤儿文件由人工清理
			return removed, freed
		}
		// 删除磁盘副本（失败不阻断：Windows 句柄占用等场景保留行，下次再试）
		if cachePath != "" {
			abs := filepath.Join(*GeneralConfig.LocalDir, filepath.FromSlash(cachePath))
			_ = os.Remove(abs)
		}
		if _, err := db.Exec(`DELETE FROM vision_cache WHERE content_hash = ? AND side = ?`, contentHash, side); err != nil {
			return removed, freed
		}
		removed++
		freed += bytes
	}
}

// migrateLegacyVisionCache 把旧版 archive.db 里的 vision_cache 表搬到 media.db。
// 幂等：旧表不存在即跳过；复制用 INSERT OR IGNORE。迁移前整库备份，失败则保留旧表。
func (d *ArchiveDB) migrateLegacyVisionCache(dbPath string) {
	if d == nil || d.db == nil {
		return
	}
	exists, err := tableExists(d.db, "vision_cache")
	if err != nil || !exists {
		return
	}
	if err := backupDatabase(d.db, dbPath, "before-db-split"); err != nil {
		logger.Error("Persistence", "视觉缓存迁移已跳过（备份失败）: %v", err)
		return
	}
	target, err := mediaDatabase.open()
	if err != nil {
		logger.Warn("Persistence", "视觉缓存迁移失败（打开媒体库）: %v", err)
		return
	}
	copied, err := copyTableRows(d.db, "vision_cache", target, "vision_cache",
		[]string{"content_hash", "side", "tag", "cache_path", "bytes", "created_at", "used_at"})
	if err != nil {
		logger.Warn("Persistence", "视觉缓存迁移失败: %v", err)
		return
	}
	if _, err := d.db.Exec(`DROP TABLE vision_cache`); err != nil {
		logger.Warn("Persistence", "视觉缓存迁移后删除旧表失败: %v", err)
		return
	}
	logger.Info("Persistence", "视觉缓存表 vision_cache 已迁入媒体派生库（%d 行）", copied)
}
