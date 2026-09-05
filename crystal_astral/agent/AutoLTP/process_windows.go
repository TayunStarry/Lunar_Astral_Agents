//go:build windows

package AutoLTP

// ==== 程序查找与启动 ====
// 负责按关键字搜索可执行程序、协议启动、启动进程、打开文件夹。

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// dtMatchProtocol 若关键字命中已知协议映射则返回对应的协议前缀。
func dtMatchProtocol(key string) string {
	for k, proto := range dtKnownProtocols {
		if strings.Contains(key, k) {
			return proto
		}
	}
	return ""
}

// DTLaunchProgram 按关键字启动程序：已开窗口则激活，否则搜索路径并启动或走协议。
func DTLaunchProgram(name string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return "", fmt.Errorf("程序名关键字为空")
	}
	for _, w := range DTListWindows() {
		if strings.Contains(strings.ToLower(w.Title), key) || strings.Contains(strings.ToLower(w.Process), key) {
			if err := DTActivateWindow(w.Title); err == nil {
				return "已激活现有窗口「" + w.Title + "」", nil
			}
		}
	}
	paths := dtSearchPrograms(key)
	if len(paths) == 0 {
		if proto := dtMatchProtocol(key); proto != "" {
			if err := exec.Command("cmd", "/c", "start", "", proto).Start(); err != nil {
				return "", fmt.Errorf("启动协议 %s 失败: %v", proto, err)
			}
			return "已通过协议启动 " + proto, nil
		}
		return "", fmt.Errorf("未找到与「%s」匹配的程序", name)
	}
	p := paths[0]
	if err := dtLaunchPath(p); err != nil {
		return "", fmt.Errorf("启动「%s」失败: %v", p, err)
	}
	return "已启动程序 " + p, nil
}

// dtSearchPrograms 在开始菜单与常见安装目录中按关键字搜索可执行/快捷方式路径。
func dtSearchPrograms(key string) []string {
	var found []string
	seen := map[string]string{}
	add := func(p string) {
		n := strings.ToLower(filepath.Base(p))
		if _, ok := seen[n]; ok {
			return
		}
		seen[n] = p
		found = append(found, p)
	}
	startDirs := []string{}
	if pd := os.Getenv("ProgramData"); pd != "" {
		startDirs = append(startDirs, filepath.Join(pd, "Microsoft", "Windows", "Start Menu", "Programs"))
	}
	if ad := os.Getenv("APPDATA"); ad != "" {
		startDirs = append(startDirs, filepath.Join(ad, "Microsoft", "Windows", "Start Menu", "Programs"))
	}
	for _, dir := range startDirs {
		filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			if ext != ".lnk" && ext != ".exe" && ext != ".bat" && ext != ".cmd" {
				return nil
			}
			if strings.Contains(strings.ToLower(strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))), key) {
				add(p)
			}
			return nil
		})
	}
	for _, dir := range []string{`C:\Program Files`, `C:\Program Files (x86)`} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				if strings.Contains(strings.ToLower(e.Name()), key) {
					sub := filepath.Join(dir, e.Name())
					subs, _ := os.ReadDir(sub)
					for _, se := range subs {
						if !se.IsDir() && strings.EqualFold(filepath.Ext(se.Name()), ".exe") {
							add(filepath.Join(sub, se.Name()))
						}
					}
				}
			} else if strings.EqualFold(filepath.Ext(e.Name()), ".exe") && strings.Contains(strings.ToLower(e.Name()), key) {
				add(filepath.Join(dir, e.Name()))
			}
		}
	}
	sort.StringSlice(found).Sort()
	return found
}

// dtLaunchPath 启动指定的可执行文件或脚本路径。
func dtLaunchPath(p string) error {
	if strings.EqualFold(filepath.Ext(p), ".exe") {
		cmd := exec.Command(p)
		cmd.Dir = filepath.Dir(p)
		return cmd.Start()
	}
	return exec.Command("cmd", "/c", "start", "", p).Start()
}

// DTOpenFolder 在文件资源管理器中打开指定文件夹绝对路径。
func DTOpenFolder(path string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", fmt.Errorf("文件夹路径为空")
	}
	if err := exec.Command("explorer.exe", p).Start(); err != nil {
		return "", fmt.Errorf("打开文件夹失败: %v", err)
	}
	return "已在文件资源管理器打开文件夹 " + p, nil
}
