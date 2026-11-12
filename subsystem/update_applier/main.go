package main

// 星月智能 · 更新器
//
// 职责边界：更新是更新器自己的事。
//   - 打包上传：枚举 → 清单 → 哈希 → 7z 分卷 → Gitee Release；
//   - 更新包查询：读取更新通道发布与分卷校验值；
//   - 下载安装：下载 → 分卷完整性/清单校验 → 等待主程序退出 → 事务写入 → 回滚兜底。
//
// 主程序只负责在启动时写出版本信标；它不再包含任何更新逻辑。

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	flag.Parse()

	root, err := filepath.Abs(*targetDir)
	if err != nil {
		pause("目标目录无效：" + err.Error())
		os.Exit(1)
	}
	if *logPath == "" {
		*logPath = filepath.Join(root, "local_data", "logs", "update_apply.log")
	}
	openLog(*logPath)

	release, ok := acquireApplyLock(root)
	if !ok {
		pause("另一个更新器实例正在运行。\n若确认没有实例在运行，请等待 15 分钟后重试（残留锁会自动失效）。")
		os.Exit(1)
	}
	defer release()

	recovered, recErr := RecoverOnStart(root)
	if recErr != nil {
		logf("[WARN] 启动恢复未完成：%v", recErr)
	}
	if recovered {
		logf("已恢复上次未完成的事务")
	}

	cfg, cfgErr := loadReleaseConfig(root)
	if cfg != nil {
		backupKeep = backupKeepCount(cfg)
	} else {
		backupKeep = defaultBackupKeep
	}

	if *headless || *packOnly || *publishNow {
		os.Exit(runHeadless(root, cfg, cfgErr))
	}

	if err := RunTUI(root, cfg, cfgErr); err != nil {
		logf("[ERROR] 界面运行失败：%v", err)
		pause("界面运行失败：" + err.Error())
		os.Exit(1)
	}
}

// runHeadless 运维模式：打包（-pack）或打包并上传（-publish）
func runHeadless(root string, cfg *ReleaseConfig, cfgErr error) int {
	if cfgErr != nil {
		logf("[ERROR] 配置加载失败：%v", cfgErr)
		fmt.Fprintln(os.Stderr, "配置加载失败："+cfgErr.Error())
		return 1
	}
	if !*packOnly && !*publishNow {
		fmt.Fprintln(os.Stderr, "用法：update_applier -headless -pack | -publish [-version v2026.11.11-1530]")
		return 2
	}

	version := strings.TrimSpace(*versionFlag)
	if version == "" {
		version = defaultTag()
	}
	report := func(p Progress) {
		if p.Message != "" {
			logf("[%s] %s", p.Phase, p.Message)
		}
	}

	var (
		res *PackResult
		err error
	)
	if *publishNow {
		res, err = PublishUpdate(context.Background(), root, cfg, version, report)
	} else {
		outDir := filepath.Join(updateRoot(root), "dist")
		res, err = BuildUpdatePackage(root, cfg, version, outDir, report)
	}
	if err != nil {
		logf("[ERROR] %v", err)
		fmt.Fprintln(os.Stderr, "失败："+err.Error())
		return 1
	}

	fmt.Printf("版本 %s · %d 个文件 · %s\n", res.Version, res.FileCount, humanSize(res.Bytes))
	for _, vol := range res.Volumes {
		name := filepath.Base(vol)
		fmt.Printf("  %s\n    SHA-256 %s\n", name, res.SHAs[name])
	}
	return 0
}

// pause 在控制台里停一下，避免双击启动时错误信息一闪而过
func pause(msg string) {
	fmt.Println(msg)
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return
	}
	fmt.Print("按回车退出…")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}
