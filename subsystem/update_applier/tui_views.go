package main

// ==== 更新器 TUI 五屏视图 ====

import (
	"fmt"
	"strconv"
	"strings"
)

// View 渲染当前帧
func (m *uiModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.showHelp {
		return m.viewHelp()
	}
	switch m.screen {
	case screenReleases:
		return m.viewReleases()
	case screenInstall:
		return m.viewInstall()
	case screenPublish:
		return m.viewPublish()
	case screenLogs:
		return m.viewLogs()
	default:
		return m.viewOverview()
	}
}

// screenMeta 当前屏的标题、上下文与底栏键位。
// 底栏与帮助页共用这一份数据（与主控制台 screenMeta 同构），保证「提示的键」就是「生效的键」。
func (m *uiModel) screenMeta() (name, context string, keys []uiKeyHint) {
	switch m.screen {
	case screenReleases:
		return "版本列表", repoLabel(m.cfg), releasesKeys()
	case screenInstall:
		ctx := ""
		if len(m.releases) > 0 {
			ctx = m.releases[m.relIdx].Tag
		}
		return "下载安装", ctx, m.installKeys()
	case screenPublish:
		return "打包上传", repoLabel(m.cfg), m.publishKeys()
	case screenLogs:
		return "日志", "", logsKeys()
	}
	return "总览", "", overviewKeys()
}

// screenFrame 按 screenMeta 构建框体并返回内容区尺寸
func (m *uiModel) screenFrame() (uiFrame, int, int) {
	name, context, keys := m.screenMeta()
	f := m.frameFor(name, context, keys)
	w, h := f.bodySize()
	return f, w, h
}

// overviewKeys 总览（等价于主控制台主菜单）：选择、进入、数字直达、帮助、退出
func overviewKeys() []uiKeyHint {
	return []uiKeyHint{
		{"↑/↓", "选择"},
		{"Enter/→", "进入"},
		{"1-5", "直达"},
		{"?", "帮助"},
		{"X", "退出"},
	}
}

// releasesKeys 版本列表
func releasesKeys() []uiKeyHint {
	return []uiKeyHint{
		{"↑/↓", "选择"},
		{"Enter/→", "去安装"},
		{"r", "刷新"},
		{"Esc", "返回总览"},
	}
}

// installKeys 下载安装（随安装状态切换提示）
func (m *uiModel) installKeys() []uiKeyHint {
	if m.installStageBusy() {
		return []uiKeyHint{{"…", "安装进行中，完成前不可操作"}}
	}
	if m.install.Pending {
		return []uiKeyHint{{"Enter", "确认"}, {"Esc", "取消确认"}}
	}
	return []uiKeyHint{
		{"Enter", "开始安装"},
		{"2", "选择版本"},
		{"Esc", "返回总览"},
	}
}

// publishKeys 打包上传（随打包/输入状态切换提示）
func (m *uiModel) publishKeys() []uiKeyHint {
	if m.publish.Busy {
		return []uiKeyHint{{"…", "打包上传中，完成前不可操作"}}
	}
	if m.publish.Editing {
		return []uiKeyHint{{"Enter", "确认版本号"}, {"Esc", "取消编辑"}}
	}
	return []uiKeyHint{
		{"Enter", "打包并上传"},
		{"e", "修改版本号"},
		{"Esc", "返回总览"},
	}
}

// logsKeys 日志
func logsKeys() []uiKeyHint {
	return []uiKeyHint{
		{"↑↓", "滚动"},
		{"Ctrl+U/Ctrl+D", "翻页"},
		{"Home/End g/G", "首 / 尾"},
		{"f", "跟随"},
		{"Esc", "返回总览"},
	}
}

// viewOverview 总览：当前版本 / 仓库 / 主程序状态 / 上次结果
func (m *uiModel) viewOverview() string {
	const keyW = 12
	f, w, h := m.screenFrame()

	rows := []string{
		sectionRule("环境", w),
		"",
		kvRow("当前版本", versionOrDash(m.curVersion), keyW),
		kvRow("目标目录", m.root, keyW),
		kvRow("仓库", repoLabel(m.cfg), keyW),
		kvRow("主程序", m.programStateText(), keyW),
		kvRow("配置文件", configStateText(m.cfg), keyW),
		"",
		sectionRule("更新", w),
		"",
		kvRow("可用版本", m.latestText(), keyW),
		kvRow("上次结果", m.lastResultText(), keyW),
		kvRow("备份保留", fmt.Sprintf("%d 次", backupKeep), keyW),
		"",
	}
	if m.install.Err != nil {
		rows = append(rows, errStyle.Render("✗ 上次安装失败："+m.install.Err.Error()))
	}
	if m.relErr != nil {
		rows = append(rows, errStyle.Render("✗ 版本列表获取失败："+m.relErr.Error()))
	}
	if len(m.logs) > 0 {
		rows = append(rows, "", sectionRule("最近日志", w), "")
		for _, line := range tail(m.logs, 3) {
			rows = append(rows, dimStyle.Render("  "+truncTo(line.Text, maxInt(8, w-4))))
		}
	}

	f.status = dimStyle.Render(fmt.Sprintf("%d 个可用版本", len(m.releases)))
	return f.render(fitLines(rows, h))
}

// viewReleases 版本列表：宽屏与主控制台主菜单一样「左列表 + 右详情」，窄屏上下排列
func (m *uiModel) viewReleases() string {
	f, w, h := m.screenFrame()

	if m.relLoading {
		return f.render(emptyState("正在获取版本列表…", "从 Gitee Release 读取更新通道的发布", "请稍候", w, h))
	}
	if m.relErr != nil {
		return f.render(emptyState("版本列表获取失败", m.relErr.Error(), "检查网络与 release.json 中的仓库配置，按 r 重试", w, h))
	}
	if len(m.releases) == 0 {
		return f.render(emptyState("暂无可用更新包", "仓库中还没有带 [UPDATE] 标记的发布", "可在「打包上传」生成第一个更新包，按 r 刷新", w, h))
	}

	twoPane := w >= 88
	listW := w
	detailW := 0
	if twoPane {
		listW = clampInt(w*45/100, 36, 50)
		detailW = w - listW - 4
	}
	rowW := maxInt(8, listW-listGutter)

	lines := []string{sectionRule("可用版本", rowW), ""}
	for i, rel := range m.releases {
		sel := i == m.relIdx
		cur := "  "
		if sel {
			cur = cursorStyle.Render("▸ ")
		}
		state := okStyle.Render("可用")
		if !rel.Valid {
			state = errStyle.Render("不完整")
		}
		relation := dimStyle.Render(relationLabel(VersionRelation(m.curVersion, rel.Tag)))
		lines = append(lines, newRow(rowW, sel).
			L(cur, valueStyle.Render(rel.Tag)).
			R(relation, "  ", state).
			String())
	}

	sel := m.releases[m.relIdx]
	detailW2 := w
	if twoPane {
		detailW2 = detailW
	}
	detail := []string{
		"",
		selectStyle.Render("✦ " + truncTo(relDisplayName(sel), maxInt(8, detailW2-4))),
		"",
		kvRow("版本", sel.Tag, 8),
		kvRow("时间", dashIfEmpty(sel.CreatedAt), 8),
		kvRow("分卷", fmt.Sprintf("%d 个 · %s", len(sel.Volumes), humanSize(assetBytes(sel))), 8),
	}
	if sel.Problem != "" {
		detail = append(detail, "", errStyle.Render("✗ "+sel.Problem))
	} else {
		detail = append(detail, "", okStyle.Render("✓ 分卷与校验值齐备"))
	}

	var body []string
	if twoPane {
		list := windowLineList(lines, m.relIdx+2, h)
		sep := ruleStyle.Render(" │ ")
		body = make([]string, 0, h)
		for i := 0; i < h; i++ {
			l, r := "", ""
			if i < len(list) {
				l = list[i]
			}
			if i < len(detail) {
				r = detail[i]
			}
			body = append(body, fitWidth(l, listW)+sep+fitWidth(r, detailW))
		}
	} else {
		listH := maxInt(3, h-8)
		body = append(body, windowLineList(lines, m.relIdx+2, listH)...)
		body = append(body, sectionRule("详情", w))
		body = append(body, detail...)
	}

	f.status = dimStyle.Render(fmt.Sprintf("%d/%d", m.relIdx+1, len(m.releases)))
	return f.render(fitLines(body, h))
}

// listGutter 列表右侧留白（避免文字贴住分隔线）
const listGutter = 2

// viewInstall 下载安装：流水线 + 双进度 + 校验结果 + 失败原因
func (m *uiModel) viewInstall() string {
	f, w, h := m.screenFrame()

	rows := []string{
		"",
		kvRow("当前版本", versionOrDash(m.curVersion), 10),
		kvRow("目标版本", versionOrDash(m.install.ToVersion), 10),
		kvRow("版本关系", relationLabel(m.selectedRelation()), 10),
		"",
	}
	if m.install.Stage == stageIdle {
		rows = append(rows, emptyState("准备安装",
			"将从 Gitee 下载全部分卷，逐个校验 SHA-256，再校验包内清单；写入前备份旧文件，失败自动回滚。",
			"按 Enter 开始；安装过程中会等待主程序退出（请先按 x 关闭主程序）。", w, maxInt(6, h-len(rows)))[1:]...)
		return f.render(fitLines(rows, h))
	}

	step := installStepIndex[m.install.Stage]
	if m.install.Err != nil {
		step = maxInt(0, step-1)
	}
	rows = append(rows, stepsBar(installSteps, step, w), "")

	label := installStageLabel(m.install.Stage)
	detail := m.install.Detail
	if detail == "" {
		detail = "—"
	}
	rows = append(rows, sectionRule(label, w), "")
	rows = append(rows, "  "+progressBar(m.install.Percent, w-2), "")
	if m.install.DownloadTotal > 0 {
		rows = append(rows, kvRow("下载量", fmt.Sprintf("%s / %s", humanSize(m.install.Downloaded), humanSize(m.install.DownloadTotal)), 10))
	}
	if m.install.FilesTotal > 0 {
		rows = append(rows, kvRow("文件", fmt.Sprintf("%d / %d", m.install.FilesDone, m.install.FilesTotal), 10))
	}
	for _, l := range wrapPlain(detail, maxInt(8, w-4)) {
		rows = append(rows, "  "+dimStyle.Render(l))
	}

	rows = append(rows, "", sectionRule("校验结果", w), "")
	rows = append(rows, m.verifyLines()...)

	if m.install.Err != nil {
		rows = append(rows, "", errStyle.Render("✗ "+truncTo(m.install.Err.Error(), maxInt(8, w-2))))
		rows = append(rows, dimStyle.Render("  建议：确认网络与 Gitee 配置；若为主程序占用，先关闭主程序后重试。"))
	}
	if m.install.Pending {
		rows = append(rows, "", warnStyle.Render("⚠ "+m.install.PendingReason+"，按 Enter 确认，Esc 取消"))
	}

	f.status = dimStyle.Render(fmt.Sprintf("%d%%", m.install.Percent))
	return f.render(fitLines(rows, h))
}

// verifyLines 校验结果区
func (m *uiModel) verifyLines() []string {
	if m.install.VerifyResult != "" {
		return []string{"  " + okStyle.Render("✓ "+m.install.VerifyResult)}
	}
	if m.install.Err != nil {
		return []string{"  " + errStyle.Render("✗ 校验未通过")}
	}
	switch m.install.Stage {
	case stageArchive, stageExtracting, stageManifest:
		return []string{"  " + dimStyle.Render("进行中…")}
	case stageBackup, stageApplying, stageCommitted, stageRolledBack:
		return []string{"  " + okStyle.Render("✓ 分卷 SHA-256、7z 完整性、包内清单逐文件校验通过")}
	}
	return []string{"  " + faintStyle.Render("尚未开始")}
}

// viewPublish 打包上传
func (m *uiModel) viewPublish() string {
	f, w, h := m.screenFrame()

	version := m.publish.Version
	versionText := valueStyle.Render(version)
	if m.publish.Editing {
		versionText = selectStyle.Render(version+"▏") + dimStyle.Render("  Enter 确认 · Esc 取消")
	}

	rows := []string{
		"",
		kvRow("版本号", "", 10) + versionText,
		kvRow("仓库", repoLabel(m.cfg), 10),
		kvRow("分卷大小", fmt.Sprintf("%d MB · 压缩等级 %d", packSizeMB(m.cfg), packLevel(m.cfg)), 10),
		kvRow("打包范围", packScopeText(m.cfg), 10),
		"",
		sectionRule("进度", w),
		"",
	}
	if m.publish.Stage == "" {
		rows = append(rows, "  "+faintStyle.Render("按 Enter 执行：枚举 → 清单 → 哈希 → 7z 分卷 → 发布说明 → 上传"))
	} else {
		rows = append(rows, "  "+progressBar(m.publish.Percent, w-2), "")
		rows = append(rows, kvRow("阶段", dashIfEmpty(publishStageLabel(m.publish.Stage)), 10))
		for _, l := range wrapPlain(dashIfEmpty(m.publish.Detail), maxInt(8, w-4)) {
			rows = append(rows, "  "+dimStyle.Render(l))
		}
	}

	if m.publish.Result != nil {
		res := m.publish.Result
		rows = append(rows, "", sectionRule("产物", w), "")
		rows = append(rows, kvRow("文件数", fmt.Sprintf("%d", res.FileCount), 10))
		rows = append(rows, kvRow("原始大小", humanSize(res.Bytes), 10))
		for _, vol := range res.Volumes {
			name := baseName(vol)
			rows = append(rows, "  "+labelStyle.Render(name))
			rows = append(rows, "    "+faintStyle.Render(truncTo(res.SHAs[name], maxInt(8, w-6))))
		}
	}
	if m.publish.Err != nil {
		rows = append(rows, "", errStyle.Render("✗ "+truncTo(m.publish.Err.Error(), maxInt(8, w-2))))
		rows = append(rows, dimStyle.Render("  建议：检查 release.json 的 gitee_token 是否具备 Release 读写权限。"))
	}

	f.status = dimStyle.Render(m.publish.StageLabel())
	return f.render(fitLines(rows, h))
}

// viewLogs 日志屏：行级滚动，follow 为真时贴底
func (m *uiModel) viewLogs() string {
	f, w, h := m.screenFrame()

	if len(m.logs) == 0 {
		return f.render(emptyState("暂无日志", "更新器运行过程会实时写入这里，并同时落盘", "执行一次下载安装或打包上传即可看到", w, h))
	}

	lines := m.wrappedLogs(w)
	maxOff := maxInt(0, len(lines)-h)
	off := m.logOffset
	if m.logFollow {
		off = maxOff
	}
	off = clampInt(off, 0, maxOff)
	body := fitLines(lines[off:minInt(off+h, len(lines))], h)

	f.status = dimStyle.Render(fmt.Sprintf("%d 行", len(m.logs)))
	return f.render(body)
}

// wrappedLogs 日志行按当前内容区宽度折行
func (m *uiModel) wrappedLogs(w int) []string {
	lineW := maxInt(8, w-2)
	body := make([]string, 0, len(m.logs))
	for _, line := range m.logs {
		style := logLineStyle(line.Text)
		for _, l := range wrapPlain(line.Text, lineW) {
			body = append(body, "  "+style.Render(l))
		}
	}
	return body
}

// logViewport 日志屏内容区尺寸（与渲染共用，保证翻页步长一致）
func (m *uiModel) logViewport() (int, int) {
	f := m.frameFor("日志", "", logsKeys())
	return f.bodySize()
}

// logPage 日志翻页步长（留一行上下文）
func (m *uiModel) logPage() int {
	_, h := m.logViewport()
	return maxInt(1, h-1)
}

// scrollLog 按行滚动日志；滚到底部自动恢复跟随
func (m *uiModel) scrollLog(delta int) {
	w, h := m.logViewport()
	total := len(m.wrappedLogs(w))
	maxOff := maxInt(0, total-h)
	off := m.logOffset
	if m.logFollow {
		off = maxOff
	}
	off = clampInt(off+delta, 0, maxOff)
	m.logOffset = off
	m.logFollow = off >= maxOff
}

// releasePage 版本列表翻页步长
func (m *uiModel) releasePage() int {
	_, _, h := m.screenFrame()
	listH := maxInt(3, h-8)
	return maxInt(1, listH-1)
}

// viewHelp 键位帮助：与主控制台一致，宽屏双栏（全局键位 / 当前屏键位），窄屏上下排列
func (m *uiModel) viewHelp() string {
	name, context, keys := m.screenMeta()
	f := m.frameFor("键位帮助", "", []uiKeyHint{{"?/Esc", "关闭"}})
	w, h := f.bodySize()

	sub := name
	if context != "" {
		sub += " · " + context
	}
	global := append([]uiKeyHint{}, uiGlobalKeys...)
	global = append(global, uiKeyHint{"侧栏", "宽屏常驻，标出当前所在屏"})

	var body []string
	if w >= 76 {
		leftW := (w - 3) / 2
		rightW := w - leftW - 3
		left := helpColumn("全局键位", global, leftW, h)
		right := helpColumn("当前屏 · "+sub, keys, rightW, h)
		sep := ruleStyle.Render(" │ ")
		for i := 0; i < h; i++ {
			l, r := "", ""
			if i < len(left) {
				l = left[i]
			}
			if i < len(right) {
				r = right[i]
			}
			body = append(body, fitWidth(l, leftW)+sep+fitWidth(r, rightW))
		}
	} else {
		body = append(body, helpColumn("全局键位", global, w, 0)...)
		body = append(body, "")
		body = append(body, helpColumn("当前屏 · "+sub, keys, w, 0)...)
		body = append(body, "", dimStyle.Render("  按 ? 或 Esc 返回"))
	}

	f.status = dimStyle.Render("帮助")
	return f.render(fitLines(body, h))
}

// ==== 文本辅助 ====

// repoLabel owner/repo
func repoLabel(cfg *ReleaseConfig) string {
	if cfg == nil || cfg.GiteeOwner == "" || cfg.GiteeRepo == "" {
		return "未配置"
	}
	return cfg.GiteeOwner + "/" + cfg.GiteeRepo
}

// configStateText 配置可用性
func configStateText(cfg *ReleaseConfig) string {
	if cfg == nil {
		return errStyle.Render("未找到 release.json")
	}
	if err := validateRepoConfig(cfg, false); err != nil {
		return errStyle.Render(err.Error())
	}
	if cfg.GiteeToken == "" {
		return warnStyle.Render("仓库已配置 · 无令牌（只能下载）")
	}
	return okStyle.Render("仓库与令牌已配置")
}

// programStateText 主程序存活状态：只认进程是否存在（每秒刷新，退出后自动回到未运行）
func (m *uiModel) programStateText() string {
	if m.cfg == nil {
		return dimStyle.Render("未知")
	}
	if m.progErr != nil {
		return warnStyle.Render("无法判定（进程枚举失败）")
	}
	if len(m.progTargets) == 0 {
		if len(m.progMissing) > 0 {
			return warnStyle.Render("未找到 " + m.progMissing[0])
		}
		return dimStyle.Render("未配置主程序文件")
	}
	if len(m.progPids) > 0 {
		pids := make([]string, 0, len(m.progPids))
		for _, pid := range m.progPids {
			pids = append(pids, strconv.Itoa(pid))
		}
		return warnStyle.Render("运行中 · PID " + strings.Join(pids, "、") + "（安装将等待退出）")
	}
	return okStyle.Render("未运行")
}

// latestText 最新可用版本
func (m *uiModel) latestText() string {
	if len(m.releases) == 0 {
		return dimStyle.Render("无")
	}
	rel := m.releases[0]
	return valueStyle.Render(rel.Tag) + "  " + dimStyle.Render(relationLabel(VersionRelation(m.curVersion, rel.Tag)))
}

// lastResultText 上次安装结果
func (m *uiModel) lastResultText() string {
	if m.state == nil {
		return dimStyle.Render("无记录")
	}
	text := fmt.Sprintf("%s · %s", dashIfEmpty(m.state.InstalledAt), dashIfEmpty(m.state.LastResult))
	if m.state.LastError != "" {
		return errStyle.Render(truncTo(text+" · "+m.state.LastError, 60))
	}
	if m.state.LastResult == "success" {
		return okStyle.Render(text)
	}
	return warnStyle.Render(text)
}

// packScopeText 打包范围摘要
func packScopeText(cfg *ReleaseConfig) string {
	if cfg == nil {
		return "—"
	}
	parts := append([]string{}, cfg.ProgramFiles...)
	parts = append(parts, cfg.Include...)
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, "、")
}

// publishStageLabel 打包阶段中文
func publishStageLabel(stage string) string {
	switch stage {
	case "manifest":
		return "生成清单与哈希"
	case "pack":
		return "7z 分卷压缩"
	case "hash_volume":
		return "计算分卷 SHA-256"
	case "publish":
		return "创建 Release"
	case "upload":
		return "上传分卷"
	case "done":
		return "完成"
	case "failed":
		return "失败"
	}
	return stage
}

// StageLabel 打包状态短标签
func (s uiPublishState) StageLabel() string {
	if s.Busy {
		return fmt.Sprintf("%d%%", s.Percent)
	}
	if s.Err != nil {
		return "失败"
	}
	if s.Result != nil {
		return "完成"
	}
	return "就绪"
}

// relDisplayName 发布名称
func relDisplayName(rel UpdateRelease) string {
	if rel.Name != "" {
		return rel.Name
	}
	return rel.Tag
}

// assetBytes 分卷总大小
func assetBytes(rel UpdateRelease) int64 {
	var total int64
	for _, a := range rel.Assets {
		total += a.Size
	}
	return total
}

// dashIfEmpty 空值占位
func dashIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// baseName 路径文件名
func baseName(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// tail 取末尾 n 项
func tail[T any](items []T, n int) []T {
	if n <= 0 || len(items) <= n {
		return items
	}
	return items[len(items)-n:]
}

// logLineStyle 日志行着色
func logLineStyle(text string) interface{ Render(...string) string } {
	switch {
	case strings.Contains(text, "[ERROR]"), strings.Contains(text, "[WARN]"):
		return errStyle
	case strings.Contains(text, "失败"):
		return errStyle
	case strings.Contains(text, "完成"), strings.Contains(text, "成功"):
		return okStyle
	}
	return dimStyle
}
