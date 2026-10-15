package main

// ==== 更新器入口 ====
//
// 与主程序同层编译（程序根目录），复用资源修复同款图标。由主程序在拉取到
// 「含主程序文件的更新包」后以运行参数拉起；不带参数直接运行（误双击等）
// 时播放失败音效并自动退出。
//
// 流程（-updatepack 指向拉取到的更新包 7z 分卷起始卷 .7z.001）：
//   1. 等待主程序退出（轮询其文件可写性，占用释放即视为已关闭）；
//   2. 用 7z 把更新包解压到程序根下的暂存目录（资源释放）；
//   3. 把暂存内容备份并覆盖到程序根目录（.backup/<时间戳>/，保留 -backup-keep 次）；
//   4. 剪枝过期备份、清理暂存目录与分卷临时目录；
//   5. 重启主程序（-restart 指定）。

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	flag.Parse()

	// 无运行参数启动：播放失败音效并自动退出（该工具不独立使用）
	if *packPath == "" {
		fmt.Fprintln(os.Stderr, "更新器须由主程序拉起（缺少 -updatepack 参数：更新包 .7z.001 文件路径）")
		playFailSound()
		os.Exit(2)
	}

	// 默认日志：程序根目录下的 local_data/logs/update_apply.log
	if *logPath == "" {
		*logPath = filepath.Join(*targetDir, "local_data", "logs", "update_apply.log")
	}
	openLog(*logPath)
	defer func() {
		if logFile != nil {
			_ = logFile.Close()
		}
	}()

	logf("更新器启动（pack=%s target=%s pid=%d）", *packPath, *targetDir, *waitPID)

	if !fileExists(*packPath) {
		logf("[ERROR] 更新包文件不存在：%s", *packPath)
		os.Exit(1)
	}

	// 单实例守卫：同一目标目录同时只允许一个更新器在跑。
	// 现实场景：用户看不到反馈时往往会再点一次程序 → 第二次拉取又拉起一个更新器，
	// 两个更新器同时覆盖同一批文件并各自重启主程序（结果是文件错乱 + 启动出两个实例）。
	if release, ok := acquireApplyLock(*targetDir); ok {
		defer release()
	} else {
		logf("[WARN] 已有另一个更新器正在处理 %s，本次不再重复执行（避免并发覆盖）", *targetDir)
		return
	}

	// 1. 等待主程序退出（占用的程序文件全部解锁）
	timeout := time.Duration(*waitTimeout) * time.Second
	self := selfPath()
	for _, rel := range waitFiles {
		p := filepath.Join(*targetDir, rel)
		if !fileExists(p) {
			continue
		}
		// 永不等待自身镜像：运行中的 exe 不允许以「写」方式打开（镜像加载器不共享写），
		// 等它只会白等到超时——实测空转 60 秒，期间用户看到的是「程序消失后毫无动静」。
		// 而覆盖自身并不需要它可写：改名/删除只需 DELETE 共享，是允许的（实测 174 个文件零跳过）。
		if samePath(p, self) {
			logf("跳过自身镜像的解锁等待：%s（覆盖自身只需可改名，无需可写）", rel)
			continue
		}
		if err := waitForUnlock(p, timeout); err != nil {
			logf("[WARN] 等待 %s 释放超时：%v（继续执行，被占用文件将跳过）", rel, err)
			continue
		}
		logf("主程序文件已释放：%s", rel)
	}

	// 2. 解压更新包到暂存目录（与程序根同卷：合并时改名落位，零拷贝）
	staging := filepath.Join(*targetDir, ".update_staging_"+time.Now().Format("20060102-150405"))
	logf("解压更新包 %s", filepath.Base(*packPath))
	if err := extractSevenZ(*packPath, staging); err != nil {
		logf("[ERROR] 解压更新包失败：%v", err)
		_ = os.RemoveAll(staging)
		os.Exit(1)
	}

	// 3. 备份并覆盖
	keep := *backupKeep
	if keep < 0 {
		keep = 0
	}
	updated, skipped, applyErr := applyTree(staging, *targetDir, keep)
	if applyErr != nil {
		logf("[ERROR] 应用更新失败：%v", applyErr)
	} else {
		logf("应用完成：已更新 %d 个文件 → %s", updated, *targetDir)
	}
	if len(skipped) > 0 {
		logf("[WARN] %d 个文件未能覆盖（被占用或失败，保留原文件）：%s", len(skipped), strings.Join(skipped, ", "))
	}

	// 4. 剪枝备份 + 清理暂存目录与分卷临时目录
	pruneBackups(*targetDir, keep)
	if err := os.RemoveAll(staging); err != nil {
		logf("[WARN] 清理暂存目录失败：%v", err)
	}
	if *packDir != "" && *packDir != *targetDir {
		if err := os.RemoveAll(*packDir); err != nil {
			logf("[WARN] 清理更新包临时目录失败：%v", err)
		}
	}

	// 5. 重启主程序
	if *restartExe != "" {
		if err := restartProgram(*restartExe, restartArgs); err != nil {
			logf("[ERROR] 重启主程序失败：%v", err)
			os.Exit(1)
		}
		logf("已重启主程序：%s", *restartExe)
	}
}
