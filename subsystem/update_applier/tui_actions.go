package main

// ==== TUI 异步动作 ====
//
// 下载安装与打包上传都在 bubbletea 的命令协程里执行：
//   - 进度通过 events 通道回送（不阻塞界面，通道满时丢弃中间帧）；
//   - 最终结果作为命令返回值回到 Update；
//   - 任何失败都保留原因并写入日志，绝不让界面停在「进行中」。

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// refreshReleases 拉取更新通道的发布列表
func (m *uiModel) refreshReleases() tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		if cfg == nil {
			return msgReleases{Err: errors.New("未找到 release.json")}
		}
		releases, err := listUpdateReleases(cfg)
		return msgReleases{Releases: releases, Err: err}
	}
}

// emit 非阻塞回送一条进度消息
func emit(events chan any, msg any) {
	if events == nil {
		return
	}
	select {
	case events <- msg:
	default:
	}
}

// startInstall 下载 → 校验 → 解压 → 清单校验 → 等待退出 → 事务写入
func (m *uiModel) startInstall() tea.Cmd {
	root := m.root
	cfg := m.cfg
	events := m.events
	cur := m.curVersion
	timeout := time.Duration(*waitTimeout) * time.Second
	if len(m.releases) == 0 {
		return func() tea.Msg { return msgInstallDone{Err: errors.New("没有可安装的版本")} }
	}
	rel := m.releases[m.relIdx]
	m.install = uiInstallState{
		Stage:       stageChecking,
		Detail:      "准备下载 " + rel.Tag,
		FromVersion: cur,
		ToVersion:   rel.Tag,
		StartedAt:   time.Now(),
	}

	return func() tea.Msg {
		detail, err := runInstall(root, cfg, &rel, cur, timeout, func(msg any) { emit(events, msg) })
		return msgInstallDone{Err: err, Detail: detail}
	}
}

// runInstall 安装流水线实体（在命令协程中运行，可安全阻塞）
func runInstall(root string, cfg *ReleaseConfig, rel *UpdateRelease, cur string, timeout time.Duration, report func(any)) (string, error) {
	if cfg == nil {
		return "", errors.New("未找到 release.json")
	}
	if rel == nil || !rel.Valid {
		return "", errors.New("发布不完整：分卷或校验值缺失")
	}

	// 1) 下载
	dir := filepath.Join(downloadRoot(root), rel.Tag)
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	report(msgInstallStage{Stage: stageDownload, Detail: "下载 " + fmt.Sprintf("%d 个分卷", len(rel.Assets))})
	if err := downloadReleaseAssets(context.Background(), cfg, rel, dir, func(p TransferProgress) {
		report(msgDownload{Progress: p})
	}); err != nil {
		return "", fmt.Errorf("下载失败：%w", err)
	}
	logf("下载完成：%s", dir)

	firstVolume := filepath.Join(dir, rel.Volumes[0])

	// 2) 分卷完整性
	report(msgInstallStage{Stage: stageArchive, Detail: "7z 完整性测试"})
	sevenZip, err := findSevenZipFor(root, cfg.SevenZip)
	if err != nil {
		return "", err
	}
	if err := testArchive(sevenZip, firstVolume); err != nil {
		return "", err
	}

	// 3) 解压
	staging := filepath.Join(stagingRoot(root), rel.Tag)
	_ = os.RemoveAll(staging)
	report(msgInstallStage{Stage: stageExtracting, Detail: "解压到 " + staging})
	if err := extractVolumes(sevenZip, firstVolume, staging); err != nil {
		return "", err
	}

	// 4) 清单校验
	report(msgInstallStage{Stage: stageManifest, Detail: "逐文件校验大小与 SHA-256"})
	files, err := verifyExtracted(staging, rel.Tag, func(p Progress) {
		report(msgInstallProgress{Progress: p})
	})
	if err != nil {
		return "", err
	}
	report(msgInstallStage{
		Stage:  stageManifest,
		Detail: "清单校验通过",
		Verify: fmt.Sprintf("%d 个文件 · %s 与清单一致", len(files), humanBytes(totalBytes(files))),
	})

	// 5) 等待主程序退出（更新器自身不在等待之列）
	report(msgInstallStage{Stage: stageWaiting, Detail: "请先关闭主程序；更新器会等待其进程退出"})
	if err := waitProgramsExit(root, cfg, timeout); err != nil {
		return "", err
	}

	// 6) 事务写入
	plan := InstallPlan{
		Target:      root,
		Staging:     staging,
		FromVersion: cur,
		ToVersion:   rel.Tag,
		Files:       files,
	}
	report(msgInstallStage{Stage: stagePrepared, Detail: "写入事务日志"})
	if err := ApplyPlan(plan, func(p Progress) {
		stage := p.Phase
		if stage == phaseBackup {
			stage = stageBackup
		} else if stage == phaseApplying {
			stage = stageApplying
		}
		report(msgInstallProgress{Progress: Progress{
			Phase:   stage,
			Done:    p.Done,
			Total:   p.Total,
			Percent: p.Percent,
			Message: p.Message,
		}})
	}); err != nil {
		return "", err
	}

	_ = os.RemoveAll(dir)

	// 7) 自动拉起主程序
	exe := mainProgramPath(root, cfg)
	if exe != "" {
		if err := restartProgram(exe, cfg.RestartArgs); err != nil {
			logf("[WARN] 自动启动主程序失败：%v", err)
			return "安装完成，但自动启动主程序失败：" + err.Error(), nil
		}
		logf("已启动主程序：%s", exe)
		return "安装完成，已启动主程序", nil
	}
	logf("[WARN] 配置中未找到可启动的主程序文件")
	return "安装完成；未找到主程序文件，请手动启动", nil
}

// startPublish 打包并上传（或按配置仅打包）
func (m *uiModel) startPublish() tea.Cmd {
	root := m.root
	cfg := m.cfg
	events := m.events
	version := m.publish.Version
	publish := *publishNow || !*packOnly
	m.publish.Busy = true
	m.publish.Stage = "manifest"
	m.publish.Detail = "准备中"
	m.publish.Percent = 0
	m.publish.Err = nil
	m.publish.Result = nil
	m.publish.StartedAt = time.Now()

	return func() tea.Msg {
		if cfg == nil {
			return msgPublishDone{Err: errors.New("未找到 release.json")}
		}
		report := func(p Progress) { emit(events, msgPackProgress{Progress: p}) }
		var (
			res *PackResult
			err error
		)
		if publish {
			res, err = PublishUpdate(context.Background(), root, cfg, version, report)
		} else {
			outDir := filepath.Join(updateRoot(root), "dist")
			res, err = BuildUpdatePackage(root, cfg, version, outDir, report)
		}
		return msgPublishDone{Result: res, Err: err}
	}
}

// refreshProgramState 重新判定主程序存活。
// 总览状态与安装等待段共用这一份判定，避免「界面显示运行中、安装却直接覆盖」这类分叉。
func (m *uiModel) refreshProgramState() {
	m.progTargets, m.progMissing = mainProgramTargets(m.root, m.cfg)
	m.progPids = nil
	m.progErr = nil
	if len(m.progTargets) == 0 {
		return
	}
	procs, err := runningProgramProcesses(m.progTargets)
	if err != nil {
		m.progErr = err
		return
	}
	for _, p := range procs {
		m.progPids = append(m.progPids, p.PID)
	}
}

// waitProgramsExit 等待配置中的主程序进程全部退出。
// 判定与总览状态同源（exe 完整路径 + PID 存活），不再依赖文件占用——
// 后者会把同目录的更新器自身当成主程序，造成空等或误跳过。
func waitProgramsExit(root string, cfg *ReleaseConfig, timeout time.Duration) error {
	targets, _ := mainProgramTargets(root, cfg)
	if len(targets) == 0 {
		return nil // 没有可等待的主程序文件：直接进入安装，不空等
	}
	deadline := time.Now().Add(timeout)
	for {
		procs, err := runningProgramProcesses(targets)
		if err != nil {
			// 枚举不可用时保守回退到占用探测：宁可等待，也不误跳过
			logf("[WARN] 进程枚举失败，改用文件占用探测：%v", err)
			return waitTargetsUnlocked(targets, timeout)
		}
		if len(procs) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("等待主程序退出超时（PID %d · %s）；请关闭主程序后重试",
				procs[0].PID, filepath.Base(procs[0].Exe))
		}
		time.Sleep(unlockPollInterval)
	}
}

// waitTargetsUnlocked 进程枚举不可用时的回退：运行中的 exe 会拒绝写入
func waitTargetsUnlocked(targets []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for _, p := range targets {
		for isFileLocked(p) {
			if time.Now().After(deadline) {
				return fmt.Errorf("等待主程序退出超时（%s）；请关闭主程序后重试", filepath.Base(p))
			}
			time.Sleep(unlockPollInterval)
		}
	}
	return nil
}

// isFileLocked 以写方式探测文件是否被占用（仅作进程枚举失败时的回退）
func isFileLocked(path string) bool {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		return true
	}
	_ = f.Close()
	return false
}

// mainProgramPath 选择需要拉起的主程序（跳过更新器自身与目录）
func mainProgramPath(root string, cfg *ReleaseConfig) string {
	targets, _ := mainProgramTargets(root, cfg)
	if len(targets) > 0 {
		return targets[0]
	}
	return ""
}

// totalBytes 文件大小合计
func totalBytes(files []FileEntry) int64 {
	var total int64
	for _, f := range files {
		total += f.Size
	}
	return total
}
