package viewerstore

// migrate.go 历史观影记忆的一次性迁移（统一记忆库 → viewer.db）。
//
// 流程：导出 JSON 备份 → 按类别解析入库 → 全部成功后才删除源条目。
// 任一步失败都不删除源条目；重复执行按幂等标记跳过，重新执行也不会重复插入。

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// errLegacyUnclassified 表示条目无法归入任何一张表（丢弃，但仍留在备份里）。
var errLegacyUnclassified = errors.New("无法归类的旧观影记忆")

// 落库接缝：测试通过替换这些变量模拟写入失败。
var (
	legacyObservationSave = SaveObservation
	legacyProfileSave     = SaveProfileAt
	legacyStanceSave      = SaveStanceAt
)

// MigrateLegacyMemories 执行历史观影记忆迁移。
//
// src 由调用方注入旧记忆库的读取与删除能力，存储层不直接依赖 engine/memory。
// 返回对账信息；失败时返回部分对账与错误，源条目不会被删除。
func MigrateLegacyMemories(src LegacyMemorySource) (MigrationReport, error) {
	report := MigrationReport{}
	if err := Init(); err != nil {
		return report, err
	}
	if MigrationDone() {
		return report, nil
	}

	scanned, err := scanLegacyMemories(src)
	if err != nil {
		// 拿不到源数据 = 迁移未完成：不标记、不删除、不写备份，等下次启动重试。
		return report, fmt.Errorf("读取历史观影记忆失败（记忆库未就绪或读取失败），迁移未完成: %w", err)
	}
	entries := selectLegacyViewerEntries(scanned)
	report.Source = len(entries)
	if report.Source == 0 {
		if err := MarkMigrationDone(); err != nil {
			return report, err
		}
		return report, nil
	}

	// 1. 先备份：备份失败时一条都不导入、不删除。
	backupPath, err := writeLegacyMemoryBackup(entries)
	if err != nil {
		return report, fmt.Errorf("导出观影记忆备份失败: %w", err)
	}
	report.BackupPath = backupPath

	// 2. 按类别解析入库。
	var firstErr error
	for _, entry := range entries {
		err := importLegacyMemoryEntry(entry)
		switch {
		case errors.Is(err, errLegacyUnclassified):
			report.Discarded++
		case err != nil:
			report.Failed++
			if firstErr == nil {
				firstErr = err
			}
		default:
			report.Imported++
		}
	}

	// 3. 导入有任何失败就一条都不删。
	if report.Failed > 0 {
		return report, fmt.Errorf("观影记忆导入失败 %d 条，源条目未删除: %w", report.Failed, firstErr)
	}

	// 4. 全部成功后才删除源条目。
	deleted := 0
	for _, entry := range entries {
		if err := deleteLegacyMemoryEntry(src, entry.ID); err != nil {
			report.Failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		deleted++
	}
	if report.Failed > 0 {
		return report, fmt.Errorf("源条目删除失败 %d 条（已删除 %d 条）: %w", report.Failed, deleted, firstErr)
	}

	if err := MarkMigrationDone(); err != nil {
		return report, err
	}
	return report, nil
}

// importLegacyMemoryEntry 把一条旧记忆按类别写进对应的表。
func importLegacyMemoryEntry(entry LegacyMemoryEntry) error {
	kind, body, ok := parseLegacyMemoryContent(entry.Content)
	if !ok || strings.TrimSpace(body) == "" {
		return errLegacyUnclassified
	}
	if LooksMechanical(entry.Content) || LooksMechanical(body) {
		return errLegacyUnclassified
	}
	switch kind {
	case "印象":
		return importLegacyObservation(body, entry.Timestamp)
	case "习惯":
		return importLegacyProfile(body, entry.Timestamp)
	case "评价", "感受":
		return importLegacyStance(body, entry.Timestamp)
	default:
		return errLegacyUnclassified
	}
}

// importLegacyObservation 把旧观感正文拆成结构化字段写入观感表（同 BV 覆盖，旧数据不覆盖新数据）。
func importLegacyObservation(body string, at int64) error {
	obs := parseLegacyObservation(body, at)
	if obs.BV == "" {
		return errLegacyUnclassified
	}
	existing, found, err := LoadObservationByBV(obs.BV)
	if err != nil {
		return err
	}
	if found && existing.UpdatedAt >= obs.UpdatedAt {
		return nil
	}
	return legacyObservationSave(obs)
}

// importLegacyProfile 把旧选片口味写入单行（已有更新的值时不回退）。
func importLegacyProfile(body string, at int64) error {
	body = stripMemoryMarker(body, SelectionMarker)
	body = Conclusion(body)
	if body == "" {
		return errLegacyUnclassified
	}
	if at == 0 {
		at = time.Now().Unix()
	}
	if _, existingAt, ok, err := LoadProfile(); err != nil {
		return err
	} else if ok && existingAt >= at {
		return nil
	}
	return legacyProfileSave(body, at)
}

// importLegacyStance 把旧评论倾向写入单行（已有更新的值时不回退）。
func importLegacyStance(body string, at int64) error {
	body = stripMemoryMarker(body, StanceMarker)
	body = Conclusion(body)
	if body == "" {
		return errLegacyUnclassified
	}
	if at == 0 {
		at = time.Now().Unix()
	}
	if _, existingAt, ok, err := LoadStance(); err != nil {
		return err
	} else if ok && existingAt >= at {
		return nil
	}
	return legacyStanceSave(body, at)
}

// scanLegacyMemories 读取旧记忆库里的全部文本条目，候选筛选交给 selectLegacyViewerEntries。
//
// 只使用通用的分页读取，不依赖观影专用读接口；读不到数据时返回错误，
// 由调用方据此判定迁移未完成，绝不把空结果当成「没有历史条目」。
func scanLegacyMemories(src LegacyMemorySource) ([]LegacyMemoryEntry, error) {
	out := make([]LegacyMemoryEntry, 0, 64)
	seen := make(map[string]bool, 64)
	const pageSize = 200
	for offset := 0; ; offset += pageSize {
		items, total, err := src.ListText(offset, pageSize)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			if item.ID == "" || seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			out = append(out, item)
		}
		if offset+len(items) >= total || len(items) < pageSize {
			break
		}
	}
	return out, nil
}

// deleteLegacyMemoryEntry 删除旧记忆库里的一条已迁移源条目。
func deleteLegacyMemoryEntry(src LegacyMemorySource, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("记忆 ID 为空")
	}
	return src.DeleteEntry(id)
}

// writeLegacyMemoryBackup 导出 JSON 备份到 local_data/database/bilibili/。
func writeLegacyMemoryBackup(entries []LegacyMemoryEntry) (string, error) {
	payload := struct {
		GeneratedAt int64               `json:"generated_at"`
		Count       int                 `json:"count"`
		Entries     []LegacyMemoryEntry `json:"entries"`
	}{
		GeneratedAt: time.Now().Unix(),
		Count:       len(entries),
		Entries:     entries,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(DataDir(), fmt.Sprintf("viewer_memory_backup_%s.json", time.Now().Format("20060102_150405")))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
