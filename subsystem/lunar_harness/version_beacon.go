package main

// ==== 版本信标 ====
//
// 主程序只负责一件事：启动时把「本次构建的版本」写到 local_data/config/version.json。
// 更新器读取该只读信标判断当前版本；主程序不查询、不下载、不安装、不回滚。
//
// 信标放在程序根目录而不是用户目录：更新器与主程序同层运行，读写同一份文件，
// 不会出现「更新器认为已装新版、主程序仍报旧版」的分叉。

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// versionBeacon 与更新器侧 VersionBeacon 字段一致（读取方在 subsystem/update_applier）。
type versionBeacon struct {
	Version string `json:"version"`
	WriteAt string `json:"write_at"`
}

// writeVersionBeacon 原子写出版本信标；buildVersion 为编译期嵌入的版本号
// （形如 2026.291.0847：年.一年中的第几天.时分）。
func writeVersionBeacon(buildVersion string) error {
	tag := beaconTag(buildVersion)
	if tag == "" {
		return errors.New("构建版本号为空，跳过本次写入")
	}
	root, err := programRoot()
	if err != nil {
		return err
	}
	path := filepath.Join(root, "local_data", "config", "version.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(versionBeacon{
		Version: tag,
		WriteAt: time.Now().Format(time.RFC3339),
	}, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// beaconTag 把构建版本号转成更新器使用的 tag 形式（vYYYY.MM.DD-HHMM）。
//
// 构建版本是「年.第几天.时分」（如 2026.291.0847），而更新包的 tag 是日历形式。
// 两者必须落在同一数字空间，版本比较才有意义，因此这里把「第几天」换算成月日。
func beaconTag(buildVersion string) string {
	raw := strings.TrimPrefix(strings.TrimSpace(buildVersion), "v")
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "v" + raw
	}
	year, errY := strconv.Atoi(parts[0])
	doy, errD := strconv.Atoi(parts[1])
	hhmm, errT := strconv.Atoi(parts[2])
	if errY != nil || errD != nil || errT != nil || year < 2000 || doy < 1 || doy > 366 || hhmm < 0 || hhmm > 2359 {
		return "v" + raw
	}
	t := time.Date(year, 1, 1, hhmm/100, hhmm%100, 0, 0, time.Local).AddDate(0, 0, doy-1)
	return t.Format("v2006.01.02-1504")
}

// programRoot 程序根目录：优先可执行文件所在目录，回退当前工作目录。
func programRoot() (string, error) {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe), nil
	}
	return os.Getwd()
}
