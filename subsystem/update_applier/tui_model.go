package main

// ==== 更新器 TUI 模型 ====

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Init 首帧：读取版本/状态已完成，这里启动日志监听、版本列表刷新与秒级时钟
func (m *uiModel) Init() tea.Cmd {
	return tea.Batch(m.listen(), m.refreshReleases(), tickCmd())
}

// tickCmd 秒级刷新（时钟、提示过期、运行时长）
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return msgTick{} })
}

// listen 等待一条异步事件
func (m *uiModel) listen() tea.Cmd {
	ch := m.events
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// Update 处理消息
func (m *uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	rearm := false
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		return m.handleKey(msg)
	case msgTick:
		// 每秒重新判定主程序存活：主程序退出后状态在一个刷新周期内回到「未运行」；
		// 同时补读日志环形缓冲，接住通道拥塞时被丢弃的日志唤醒。
		m.refreshProgramState()
		m.drainLogs()
		return m, tickCmd()
	case msgLog:
		m.drainLogs()
		rearm = true
	case msgReleases:
		m.relLoading = false
		m.relErr = msg.Err
		if msg.Err == nil {
			m.releases = msg.Releases
			m.relIdx = clampInt(m.relIdx, 0, maxInt(0, len(m.releases)-1))
			m.setNotice(fmt.Sprintf("版本列表已更新（%d 个可用版本）", len(m.releases)), false)
		} else {
			m.setNotice("版本列表获取失败", true)
		}
		rearm = true
	case msgInstallStage:
		m.install.Stage = msg.Stage
		m.install.Detail = msg.Detail
		if msg.Percent > 0 {
			m.install.Percent = msg.Percent
		}
		if msg.Verify != "" {
			m.install.VerifyResult = msg.Verify
		}
		rearm = true
	case msgDownload:
		p := msg.Progress
		m.install.Stage = stageDownload
		m.install.Detail = p.Message
		m.install.Percent = p.Percent
		m.install.Downloaded = p.Done
		m.install.DownloadTotal = p.Total
		rearm = true
	case msgInstallProgress:
		p := msg.Progress
		if p.Phase != "" {
			m.install.Stage = p.Phase
		}
		if p.Message != "" {
			m.install.Detail = p.Message
		}
		m.install.Percent = p.Percent
		m.install.FilesDone = p.Done
		m.install.FilesTotal = p.Total
		rearm = true
	case msgInstallDone:
		m.install.Err = msg.Err
		m.install.Detail = msg.Detail
		if msg.Err == nil {
			m.install.Stage = stageCommitted
			m.install.Percent = 100
			m.setNotice("安装完成", false)
		} else {
			m.install.Stage = stageFailed
			m.setNotice("安装失败", true)
		}
		// 安装完成会拉起主程序，立即刷新一次，避免状态停在安装前的旧值
		m.refreshProgramState()
		rearm = true
	case msgPackProgress:
		p := msg.Progress
		m.publish.Stage = p.Phase
		m.publish.Detail = p.Message
		m.publish.Percent = clampInt(p.Percent, 0, 100)
		if p.State != "" {
			m.applyVolumeProgress(p)
		}
		rearm = true
	case msgPublishDone:
		m.publish.Busy = false
		m.publish.Err = msg.Err
		m.publish.Result = msg.Result
		m.syncVolumes(msg.Result, msg.Err)
		if msg.Err == nil && msg.Result != nil {
			m.publish.Stage = "done"
			m.publish.Percent = 100
			if publishMode() {
				m.publish.Detail = fmt.Sprintf("已上传 %d 个分卷", len(msg.Result.Volumes))
			} else {
				m.publish.Detail = fmt.Sprintf("已产出 %d 个分卷（未上传）", len(msg.Result.Volumes))
			}
			m.setNotice("打包上传完成", false)
			cmds = append(cmds, m.refreshReleases())
		} else if msg.Err == nil {
			m.publish.Stage = "failed"
			m.publish.Err = fmt.Errorf("打包结果为空")
			m.publish.ErrStage = "pack"
			m.setNotice("打包上传失败", true)
		} else {
			if m.publish.ErrStage == "" {
				m.publish.ErrStage = m.publish.Stage
			}
			m.publish.Stage = "failed"
			m.setNotice("打包上传失败", true)
		}
		rearm = true
	}

	if rearm {
		cmds = append(cmds, m.listen())
	}
	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

// handleKey 键位分发：与主控制台 engine/console 的按键约定逐条对齐
func (m *uiModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// 退出确认优先（仅在安装写入阶段由 x/X 触发）
	if m.confirmQuit {
		switch strings.ToLower(key) {
		case "y":
			m.confirmQuit = false
			return m, tea.Quit
		case "n", "esc":
			m.confirmQuit = false
			m.setNotice("已取消退出", false)
		}
		return m, nil
	}

	// Ctrl+C 强制退出：与主控制台一致，任何状态都立即退出
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	// 版本号输入期间，按键交给输入框（x、?、[ ] 都作为正文字符让位）
	if m.publish.Editing {
		return m.handlePublishEdit(msg)
	}

	// 帮助页为模态：只认关闭键
	if m.showHelp {
		if key == "?" || key == "esc" {
			m.showHelp = false
		}
		return m, nil
	}

	switch key {
	case "x", "X":
		if m.install.Guarded() {
			m.confirmQuit = true
			m.setNotice("安装正在写入，退出需确认（y 确认 / n 取消）", true)
			return m, nil
		}
		return m, tea.Quit
	case "?":
		m.showHelp = true
	case "esc":
		// 先取消二次确认；再返回总览（等价于主控制台的 Esc 返回主菜单）
		if m.install.Pending {
			m.install.Pending = false
			return m, nil
		}
		m.screen = screenOverview
		m.cursor = screenOverview
	case "[", "pgup":
		m.switchScreen(-1)
	case "]", "pgdown":
		m.switchScreen(1)
	case "1", "2", "3", "4", "5":
		m.gotoScreen(int(key[0] - '1'))
	case "enter":
		return m.handleEnter()
	case "right":
		// → 只在总览（菜单等价屏）表示进入，避免在功能屏误触发安装 / 打包
		if m.screen == screenOverview {
			return m.handleEnter()
		}
	case "up", "k":
		if m.screen == screenLogs {
			m.scrollLog(-1)
		} else {
			m.move(-1)
		}
	case "down", "j":
		if m.screen == screenLogs {
			m.scrollLog(1)
		} else {
			m.move(1)
		}
	case "home", "g":
		m.jumpFirst()
	case "end", "G":
		m.jumpLast()
	case "ctrl+u":
		m.pageBack()
	case "ctrl+d":
		m.pageForward()
	case "f":
		if m.screen == screenLogs {
			if m.logFollow {
				// 关闭跟随时停在当前底部，避免瞬间跳到顶部
				w, h := m.logViewport()
				m.logOffset = maxInt(0, len(m.wrappedLogs(w))-h)
				m.logFollow = false
			} else {
				m.logFollow = true
			}
		}
	case "e":
		if m.screen == screenPublish && !m.publish.Busy {
			m.publish.Editing = true
		}
	case "r":
		if m.screen == screenReleases && !m.relLoading {
			m.relLoading = true
			m.setNotice("正在刷新版本列表…", false)
			return m, m.refreshReleases()
		}
	}
	return m, nil
}

// handleEnter 各屏的确认动作（Enter 与 → 同义）
func (m *uiModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenOverview:
		m.screen = clampInt(m.cursor, 0, screenCount-1)
		m.cursor = m.screen
	case screenReleases:
		if len(m.releases) == 0 {
			m.setNotice("没有可用版本，按 r 刷新", true)
			return m, nil
		}
		m.screen = screenInstall
		m.cursor = screenInstall
		m.prepareInstallTarget()
	case screenInstall:
		if m.install.Pending {
			m.install.Pending = false
			return m, m.startInstall()
		}
		if m.installStageBusy() {
			return m, nil
		}
		if len(m.releases) == 0 {
			m.setNotice("请先在「版本列表」选择版本", true)
			return m, nil
		}
		m.prepareInstallTarget()
		rel := m.selectedRelation()
		if rel == "older" || rel == "same" {
			m.install.Pending = true
			if rel == "older" {
				m.install.PendingReason = "目标版本低于当前版本，将执行降级"
			} else {
				m.install.PendingReason = "目标版本与当前版本相同，将执行重装"
			}
			return m, nil
		}
		return m, m.startInstall()
	case screenPublish:
		if m.publish.Busy {
			return m, nil
		}
		if m.publish.Version == "" {
			m.publish.Version = defaultTag()
		}
		return m, m.startPublish()
	}
	return m, nil
}

// handlePublishEdit 版本号输入
func (m *uiModel) handlePublishEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.publish.Editing = false
		if strings.TrimSpace(m.publish.Version) == "" {
			m.publish.Version = defaultTag()
		}
	case tea.KeyEsc:
		m.publish.Editing = false
	case tea.KeyBackspace:
		r := []rune(m.publish.Version)
		if len(r) > 0 {
			m.publish.Version = string(r[:len(r)-1])
		}
	case tea.KeyRunes:
		for _, r := range msg.Runes {
			if strings.ContainsRune("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.-_", r) {
				m.publish.Version += string(r)
			}
		}
	case tea.KeySpace:
		m.publish.Version += " "
	}
	return m, nil
}

// move ↑/↓ k/j：总览移动选屏游标，版本列表移动版本游标，其余屏无列表可移
func (m *uiModel) move(delta int) {
	switch m.screen {
	case screenOverview:
		m.cursor = clampInt(m.cursor+delta, 0, screenCount-1)
	case screenReleases:
		if len(m.releases) > 0 {
			m.relIdx = clampInt(m.relIdx+delta, 0, len(m.releases)-1)
		}
	}
}

// gotoScreen 数字直达 / 侧栏切换
func (m *uiModel) gotoScreen(idx int) {
	idx = clampInt(idx, 0, screenCount-1)
	m.screen = idx
	m.cursor = idx
}

// switchScreen 按侧栏顺序切上一屏 / 下一屏（[] 与 PgUp / PgDn）
func (m *uiModel) switchScreen(delta int) {
	m.gotoScreen((m.screen + delta + screenCount) % screenCount)
}

// jumpFirst Home / g：列表到首项，日志到顶部
func (m *uiModel) jumpFirst() {
	switch m.screen {
	case screenLogs:
		m.logFollow = false
		m.logOffset = 0
	case screenReleases:
		m.relIdx = 0
	case screenOverview:
		m.cursor = 0
	}
}

// jumpLast End / G：列表到末项，日志贴底
func (m *uiModel) jumpLast() {
	switch m.screen {
	case screenLogs:
		m.logFollow = true
	case screenReleases:
		m.relIdx = maxInt(0, len(m.releases)-1)
	case screenOverview:
		m.cursor = screenCount - 1
	}
}

// pageBack Ctrl+U：屏内向上翻页
func (m *uiModel) pageBack() {
	switch m.screen {
	case screenLogs:
		m.scrollLog(-m.logPage())
	case screenReleases:
		m.relIdx = clampInt(m.relIdx-m.releasePage(), 0, maxInt(0, len(m.releases)-1))
	}
}

// pageForward Ctrl+D：屏内向下翻页
func (m *uiModel) pageForward() {
	switch m.screen {
	case screenLogs:
		m.scrollLog(m.logPage())
	case screenReleases:
		m.relIdx = clampInt(m.relIdx+m.releasePage(), 0, maxInt(0, len(m.releases)-1))
	}
}

// prepareInstallTarget 把选中版本带入安装屏
func (m *uiModel) prepareInstallTarget() {
	if len(m.releases) == 0 {
		return
	}
	rel := m.releases[m.relIdx]
	if m.install.ToVersion != rel.Tag {
		m.install = uiInstallState{Stage: stageIdle, FromVersion: m.curVersion, ToVersion: rel.Tag}
	}
}

// selectedRelation 当前版本与选中版本的关系
func (m *uiModel) selectedRelation() string {
	if len(m.releases) == 0 {
		return "unknown"
	}
	return VersionRelation(m.curVersion, m.releases[m.relIdx].Tag)
}

// installStageBusy 安装流水线是否正在推进
func (m *uiModel) installStageBusy() bool {
	switch m.install.Stage {
	case stageChecking, stageDownload, stageArchive, stageExtracting, stageManifest, stageWaiting, stagePrepared, stageBackup, stageApplying:
		return true
	}
	return false
}

// Guarded 是否已进入不可安全中断的写入阶段
func (s uiInstallState) Guarded() bool {
	return s.Stage == stageBackup || s.Stage == stageApplying || s.Stage == phaseApplying
}

// appendLog 追加界面日志
func (m *uiModel) appendLog(line string) {
	m.logs = append(m.logs, uiLogLine{At: time.Now(), Text: line})
	if len(m.logs) > uiLogLimit {
		m.logs = m.logs[len(m.logs)-uiLogLimit:]
	}
}

// drainLogs 把内存环形缓冲里尚未显示的行补进界面日志。
// 与日志文件同源：即使 events 通道拥塞丢了唤醒信号，下一轮 tick 也会补齐。
func (m *uiModel) drainLogs() {
	entries, last := readLogRing(m.logSeq)
	for _, e := range entries {
		m.appendLog(e.Line)
	}
	m.logSeq = last
}

// applyVolumeProgress 用进度消息更新单卷状态（逐卷上传结果）
func (m *uiModel) applyVolumeProgress(p Progress) {
	total := p.Total
	if total <= 0 {
		total = p.Item + 1
	}
	for len(m.publish.Volumes) < total {
		m.publish.Volumes = append(m.publish.Volumes, uiUploadVolume{State: "pending"})
	}
	if p.Item < 0 || p.Item >= len(m.publish.Volumes) {
		return
	}
	v := &m.publish.Volumes[p.Item]
	if p.Message != "" && v.Name == "" {
		v.Name = p.Message
	}
	if p.ItemSize > 0 {
		v.Size = p.ItemSize
	}
	if p.State != "" {
		v.State = p.State
	}
	if p.Detail != "" {
		v.Detail = p.Detail
	}
}

// syncVolumes 打包结束后用产物补齐卷名，并把未回送状态（进度帧被丢弃）补成确定值
func (m *uiModel) syncVolumes(res *PackResult, err error) {
	if res != nil {
		for len(m.publish.Volumes) < len(res.Volumes) {
			m.publish.Volumes = append(m.publish.Volumes, uiUploadVolume{State: "pending"})
		}
		for i, vol := range res.Volumes {
			if i >= len(m.publish.Volumes) {
				break
			}
			if m.publish.Volumes[i].Name == "" {
				m.publish.Volumes[i].Name = baseName(vol)
			}
		}
	}
	if err != nil {
		// 没有明确失败卷时，按顺序把第一个未完成卷标成失败（上传是串行的）
		marked := false
		for i := range m.publish.Volumes {
			if m.publish.Volumes[i].State == "failed" {
				marked = true
			}
		}
		if !marked {
			for i := range m.publish.Volumes {
				if m.publish.Volumes[i].State == "uploading" || m.publish.Volumes[i].State == "pending" {
					m.publish.Volumes[i].State = "failed"
					if m.publish.Volumes[i].Detail == "" {
						m.publish.Volumes[i].Detail = truncTo(err.Error(), 80)
					}
					break
				}
			}
		}
		return
	}
	for i := range m.publish.Volumes {
		if m.publish.Volumes[i].State != "failed" {
			m.publish.Volumes[i].State = "done"
		}
	}
}

// setNotice 设置标题栏结果提示
func (m *uiModel) setNotice(text string, isErr bool) {
	m.notice = text
	m.noticeErr = isErr
	m.noticeAt = time.Now()
}

// versionOrDash 版本号空值显示
func versionOrDash(v string) string {
	if v == "" {
		return "未知"
	}
	return v
}

// installStageLabel 阶段中文名
func installStageLabel(stage string) string {
	switch stage {
	case stageChecking:
		return "查询版本"
	case stageDownload:
		return "下载分卷"
	case stageArchive:
		return "校验分卷"
	case stageExtracting:
		return "解压"
	case stageManifest:
		return "清单校验"
	case stageWaiting:
		return "等待主程序退出"
	case stagePrepared:
		return "写入事务日志"
	case stageBackup:
		return "备份旧文件"
	case stageApplying:
		return "覆盖新文件"
	case stageCommitted:
		return "安装完成"
	case stageRolledBack:
		return "已回滚"
	case stageFailed:
		return "失败"
	}
	return "就绪"
}
