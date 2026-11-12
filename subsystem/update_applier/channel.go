package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ==== 更新通道与分卷解析 ====

const (
	// updateMarker 更新包 Release 描述的通道标记行首
	updateMarker = "[UPDATE]"
	// updateAssetPrefix 更新包分卷附件名前缀
	updateAssetPrefix = "lunar_updatepack_"
	// volumeSuffix 分卷后缀（.7z.001 起）
	volumeSuffix = ".7z."
)

// isUpdateAsset 判断附件是否属于更新包通道。
func isUpdateAsset(name string) bool {
	return strings.HasPrefix(name, updateAssetPrefix) && strings.Contains(name, volumeSuffix)
}

// isUpdateRelease 判断发布是否属于更新包通道。
func isUpdateRelease(r GiteeRelease) bool {
	if strings.Contains(r.Body, updateMarker) {
		return true
	}
	for _, a := range r.Assets {
		if isUpdateAsset(a.Name) {
			return true
		}
	}
	return false
}

// parseDeclaredSHAs 从发行说明解析「附件名 → SHA-256」映射。
// 约定格式：
//
//	- lunar_updatepack_x.7z.001（95.0 MB）
//	  SHA-256: <hex>
func parseDeclaredSHAs(body string) map[string]string {
	out := make(map[string]string)
	current := ""
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "- ") {
			name := strings.TrimPrefix(line, "- ")
			if idx := strings.IndexAny(name, "（("); idx >= 0 {
				name = name[:idx]
			}
			current = strings.TrimSpace(name)
			continue
		}
		if current == "" {
			continue
		}
		if idx := strings.Index(strings.ToUpper(line), "SHA-256:"); idx >= 0 {
			sum := strings.TrimSpace(line[idx+len("SHA-256:"):])
			if sum != "" {
				out[current] = strings.ToLower(sum)
			}
			current = ""
		}
	}
	return out
}

// volumeIndex 解析分卷编号（.7z.001 → 1），非分卷返回 0。
func volumeIndex(name string) int {
	idx := strings.LastIndex(name, volumeSuffix)
	if idx < 0 {
		return 0
	}
	n, err := strconv.Atoi(name[idx+len(volumeSuffix):])
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// orderedVolumes 返回按编号排序的分卷名，并校验从 001 起连续。
func orderedVolumes(assets []GiteeAsset) ([]string, error) {
	type vol struct {
		name string
		idx  int
	}
	var vols []vol
	for _, a := range assets {
		if !isUpdateAsset(a.Name) {
			continue
		}
		idx := volumeIndex(a.Name)
		if idx == 0 {
			continue
		}
		vols = append(vols, vol{name: a.Name, idx: idx})
	}
	if len(vols) == 0 {
		return nil, fmt.Errorf("发布中没有更新包分卷")
	}
	sort.Slice(vols, func(i, j int) bool { return vols[i].idx < vols[j].idx })
	names := make([]string, 0, len(vols))
	for i, v := range vols {
		if v.idx != i+1 {
			return nil, fmt.Errorf("分卷不连续：期望第 %d 卷，实际 %s", i+1, v.name)
		}
		names = append(names, v.name)
	}
	return names, nil
}
