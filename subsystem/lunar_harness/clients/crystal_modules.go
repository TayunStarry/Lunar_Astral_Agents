package clients

import (
	"io/fs"
	"path"
)

// 内置琉璃模块的存放约定：clients/crystal/<模块目录>/metadata.json
const (
	crystalDir          = "crystal"
	crystalMetadataFile = "metadata.json"
)

// CrystalModule 内嵌在琉璃桌面里的内置模块条目：目录名（即包名）与 metadata.json 原文。
type CrystalModule struct {
	Name     string // 模块目录名（clients/crystal/<name>/）
	Metadata []byte // metadata.json 原始内容，由调用方按需解析
}

// CrystalModules 枚举内嵌琉璃桌面中的内置模块，返回目录名与 metadata.json 原文。
//
// 内置模块随二进制发布在 clients/crystal/<name>/metadata.json，不占 local_data/package；
// 缺少 metadata.json 的子目录（纯资源目录）跳过，返回顺序按目录名升序（fs.ReadDir 保证）。
// 模块清单（/api/packages 展示与 AtoA 工具链扫描）统一经本函数读取，
// 避免两处各自扫描后出现「同一份内置包清单不一致」的漂移。
func CrystalModules() []CrystalModule {
	return crystalModules(EmbeddedFiles)
}

// crystalModules 可注入文件系统的实现，供测试覆盖排序与缺失目录行为。
func crystalModules(fsys fs.FS) []CrystalModule {
	entries, err := fs.ReadDir(fsys, crystalDir)
	if err != nil {
		return nil
	}
	out := make([]CrystalModule, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath := path.Join(crystalDir, entry.Name(), crystalMetadataFile)
		data, err := fs.ReadFile(fsys, metaPath)
		if err != nil {
			continue
		}
		out = append(out, CrystalModule{Name: entry.Name(), Metadata: data})
	}
	return out
}
