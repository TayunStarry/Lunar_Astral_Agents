package resource

// cache.go 统一缓存表：视频抽帧缓存 + 图片缩影缓存与源文件 URL 的映射
//
// 主键 (kind, source_url, variant)：
//   - image-thumbnail 的 variant 为缩放边长；
//   - video-slice 的 variant 为 segN（第 N 个 60 秒切片）。
//
// 缓存产物真实落盘（multimedia/vision、multimedia/footage），表只做簿记与 LRU。

import (
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// cacheVariant 生成缓存变体键
func cacheVariant(base string, number int) string {
	return base + strconv.Itoa(number)
}

// cacheGet 读取一条缓存记录
func (d *store) cacheGet(kind, sourceURL, variant string) (CacheEntry, bool) {
	var entry CacheEntry
	entry.Kind, entry.SourceURL, entry.Variant = kind, NormalizeURL(sourceURL), variant
	err := d.db.QueryRow(`SELECT path, bytes FROM cache WHERE kind = ? AND source_url = ? AND variant = ?`,
		entry.Kind, entry.SourceURL, variant).Scan(&entry.Path, &entry.Bytes)
	if err != nil {
		return CacheEntry{}, false
	}
	return entry, true
}

// cachePut 写入 / 覆写一条缓存记录
func (d *store) cachePut(entry CacheEntry) error {
	now := time.Now().Unix()
	_, err := d.db.Exec(`INSERT INTO cache (kind, source_url, variant, path, bytes, created_at, used_at)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(kind, source_url, variant) DO UPDATE SET
			path = excluded.path, bytes = excluded.bytes, used_at = excluded.used_at`,
		entry.Kind, NormalizeURL(entry.SourceURL), entry.Variant, entry.Path, entry.Bytes, now, now)
	return err
}

// cacheTouch 缓存命中续期
func (d *store) cacheTouch(kind, sourceURL, variant string) {
	_, _ = d.db.Exec(`UPDATE cache SET used_at = ? WHERE kind = ? AND source_url = ? AND variant = ?`,
		time.Now().Unix(), kind, NormalizeURL(sourceURL), variant)
}

// cacheEvict 容量超限时按 LRU 淘汰（连簿记行与磁盘文件一起清理）
func (d *store) cacheEvict(maxBytes int64) (int, int64) {
	if maxBytes <= 0 {
		return 0, 0
	}
	removed, freed := 0, int64(0)
	for {
		var total int64
		if err := d.db.QueryRow(`SELECT COALESCE(SUM(bytes),0) FROM cache`).Scan(&total); err != nil {
			return removed, freed
		}
		if total <= maxBytes {
			return removed, freed
		}
		row := d.db.QueryRow(`SELECT kind, source_url, variant, path, bytes FROM cache
			ORDER BY used_at ASC, created_at ASC LIMIT 1`)
		var kind, sourceURL, variant, cachePath string
		var bytes int64
		if err := row.Scan(&kind, &sourceURL, &variant, &cachePath, &bytes); err != nil {
			if err == sql.ErrNoRows {
				return removed, freed
			}
			return removed, freed
		}
		if cachePath != "" {
			_ = os.Remove(filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(cachePath)))
		}
		if _, err := d.db.Exec(`DELETE FROM cache WHERE kind = ? AND source_url = ? AND variant = ?`,
			kind, sourceURL, variant); err != nil {
			return removed, freed
		}
		removed++
		freed += bytes
	}
}

// VisionCacheGet 按源文件 URL 与边长查图片缩影
func VisionCacheGet(sourceURL string, side int) (VisionCacheEntry, bool) {
	if EnsureReady() != nil {
		return VisionCacheEntry{}, false
	}
	entry, ok := archiveDB.cacheGet(CacheKindImageThumb, sourceURL, cacheVariant("s", side))
	if !ok {
		return VisionCacheEntry{}, false
	}
	out := VisionCacheEntry{
		SourceURL: entry.SourceURL, Side: side, CachePath: entry.Path, Bytes: entry.Bytes,
	}
	if res, found := archiveDB.FindResourceByURL(sourceURL); found {
		out.Tag = res.Handle
	}
	return out, true
}

// VisionCachePut 写入图片缩影缓存
func VisionCachePut(entry VisionCacheEntry) error {
	if EnsureReady() != nil {
		return nil
	}
	return archiveDB.cachePut(CacheEntry{
		Kind: CacheKindImageThumb, SourceURL: entry.SourceURL,
		Variant: cacheVariant("s", entry.Side), Path: entry.CachePath, Bytes: entry.Bytes,
	})
}

// VisionCacheTouch 图片缩影命中续期
func VisionCacheTouch(sourceURL string, side int) {
	if EnsureReady() != nil {
		return
	}
	archiveDB.cacheTouch(CacheKindImageThumb, sourceURL, cacheVariant("s", side))
}

// VisionCacheEvict 缓存池超限时按 LRU 淘汰
func VisionCacheEvict(maxBytes int64) (int, int64) {
	if EnsureReady() != nil {
		return 0, 0
	}
	return archiveDB.cacheEvict(maxBytes)
}

// SliceCacheGet 读取某视频第 seg 片的抽帧缓存目录
func SliceCacheGet(sourceURL string, seg int) (string, bool) {
	if EnsureReady() != nil {
		return "", false
	}
	entry, ok := archiveDB.cacheGet(CacheKindVideoSlice, sourceURL, cacheVariant("seg", seg))
	if !ok || entry.Path == "" {
		return "", false
	}
	archiveDB.cacheTouch(CacheKindVideoSlice, sourceURL, entry.Variant)
	return entry.Path, true
}

// SliceCachePut 写入一张切片缓存（dir 为存放 JPEG 抽帧的相对目录）
func SliceCachePut(sourceURL string, seg int, dir string, bytes int64) error {
	if EnsureReady() != nil || dir == "" {
		return nil
	}
	return archiveDB.cachePut(CacheEntry{
		Kind: CacheKindVideoSlice, SourceURL: sourceURL,
		Variant: cacheVariant("seg", seg), Path: dir, Bytes: bytes,
	})
}
