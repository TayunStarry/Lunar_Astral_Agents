package library

// metadata.go 非文本文件详情字段：入库时间 / 句柄 / 引用积分 / 描述状态。

import (
	"fmt"
	"os"
	"strings"

	"LunarSubsystem/LunarHarness/engine/resource"
)

// MetaOf 读取单个文件的固定详情字段。
func MetaOf(rel string) (Metadata, error) {
	rel = normalizeRel(rel)
	full, err := resolve(rel)
	if err != nil {
		return Metadata{}, err
	}
	info, statErr := os.Stat(full)
	if statErr != nil {
		return Metadata{}, fmt.Errorf("文件不存在")
	}
	if info.IsDir() {
		return Metadata{}, fmt.Errorf("目录没有文件详情")
	}
	meta := Metadata{
		Path:         rel,
		Name:         info.Name(),
		Ext:          extOf(rel),
		Size:         info.Size(),
		MTime:        info.ModTime().Unix(),
		GCCycleHours: resource.GCCycleHours,
	}
	res, ok := resource.ForPath(rel)
	if !ok {
		return meta, nil
	}
	meta.Handle = res.Handle
	meta.Kind = res.Kind
	meta.CreatedAt = res.CreatedAt
	meta.MIME = res.MIME
	meta.State = res.State
	meta.Summary = res.Summary
	meta.HasDescription = strings.TrimSpace(res.Summary) != "" || res.ParseState == resource.ParseReady
	meta.Points = resource.PointsOf(res.Handle)
	meta.GCTracked = resource.GCTrackedPath(res.BlobPath, res.Kind)
	meta.GCProtected = strings.TrimSpace(res.ParseText) != ""
	if meta.GCTracked {
		meta.GCCyclesLeft = resource.GCCyclesLeft(meta.Points)
	}
	return meta, nil
}
