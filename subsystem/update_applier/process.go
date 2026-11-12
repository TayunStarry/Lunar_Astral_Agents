package main

// ==== 主程序存活判定 ====
//
// 判定依据只有一条：目标 exe 是否真的有进程在运行。
// 不再用「文件能否以写方式打开」推断：更新器与主程序同目录，程序文件清单里也包含
// 更新器自身，占用探测会把正在运行的更新器误当成主程序（表现为「没开主程序却显示运行中」）。
//
// 判定流程（与安装等待段共用）：
//  1. 从 release.json 的 program_files 中筛出真实存在、且不是更新器自身的主程序文件；
//  2. 枚举进程，按可执行文件完整路径命中这些文件；
//  3. 再用 PID 复核进程仍存活（快照里可能残留已退出条目）。

import (
	"os"
	"path/filepath"
	"strings"
)

// normalizePath 路径归一（Windows 下大小写不敏感）
func normalizePath(p string) string {
	return strings.ToLower(filepath.Clean(p))
}

// mainProgramTargets 从配置中筛出真实存在的主程序文件，跳过更新器自身与目录。
// missing 保留配置里写了但不存在的相对路径，供界面明确提示「未找到」。
func mainProgramTargets(root string, cfg *ReleaseConfig) (targets, missing []string) {
	if cfg == nil {
		return nil, nil
	}
	self := selfPath()
	for _, rel := range cfg.ProgramFiles {
		p, err := safeJoin(root, rel)
		if err != nil {
			continue
		}
		if samePath(p, self) {
			continue
		}
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			missing = append(missing, rel)
			continue
		}
		targets = append(targets, p)
	}
	return targets, missing
}

// matchTarget 判断进程的可执行文件路径是否命中待检测的主程序文件。
// 先比字符串（大小写归一），再用 os.SameFile 兜住 8.3 短路径与符号链接。
func matchTarget(exe string, targets []string) bool {
	if exe == "" {
		return false
	}
	for _, t := range targets {
		if normalizePath(exe) == normalizePath(t) || samePath(exe, t) {
			return true
		}
	}
	return false
}
