package console

// view_update_action.go 更新包屏的操作提交、后台命令与标题栏文案。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"LunarSubsystem/LunarHarness/engine/console/updatepack"
)

// submitUpdateInput 输入态回车提交（值接收者：修改随返回的模型副本生效）
func (m appModel) submitUpdateInput() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(m.update.input.Value())
	if value == "" {
		m.update.mode = updateMenuMode
		m.update.input.Blur()
		return m, nil
	}
	m.update.running = true
	m.update.progress = 0
	m.update.err = nil
	m.update.input.Blur() // 提交后离开输入态
	switch m.update.mode {
	case updateInputTag:
		m.update.runningOp = "拉取更新包（tag: " + value + "）"
		m.setBusy(m.update.runningOp)
		return m, runUpdateCmd("tag", value)
	case updateInputToken:
		// 令牌由发布时手动输入：配置文件不再保存密钥。
		// 同时记入会话：后续列表/拉取请求都会带上它，避免一直吃匿名额度（限流）
		SetSessionToken(value)
		m.update.runningOp = "发布更新包"
		m.setBusy(m.update.runningOp)
		return m, runUpdateCmd("publish", value)
	}
	m.update.runningOp = "拉取更新包（commit: " + value + "）"
	m.setBusy(m.update.runningOp)
	return m, runUpdateCmd("hash", value)
}

// updateMenuAction 执行操作菜单当前选中项
func (m appModel) updateMenuAction() (tea.Model, tea.Cmd) {
	idx := clampInt(m.update.menuIdx, 0, len(updateMenuEntries)-1)
	switch idx {
	case 0:
		// 发布需 Gitee 私人令牌：先进入令牌输入态（不回显明文）
		// 配置文件 config/release.json 已有令牌时预填，直接回车即可发布
		m.update.mode = updateInputToken
		m.update.input.EchoMode = textinput.EchoPassword
		m.update.input.SetValue(configuredGiteeToken())
		m.update.input.Placeholder = "输入 Gitee 私人令牌（需 Release 读写权限）"
		m.update.input.Focus() // 必须 Focus：未聚焦时 textinput 直接丢弃按键
		m.update.err = nil
		return m, textinput.Blink
	case 1:
		m.update.running = true
		m.update.progress = 0
		m.update.err = nil
		m.update.runningOp = "拉取最新更新包"
		m.setBusy(m.update.runningOp)
		return m, runUpdateCmd("latest", "")
	case 2:
		m.update.mode = updateInputTag
		m.update.input.EchoMode = textinput.EchoNormal
		m.update.input.SetValue("")
		m.update.input.Placeholder = "例如 v2026.10.14 或 v1.2.0"
		m.update.input.Focus()
		return m, textinput.Blink
	case 3:
		m.update.mode = updateInputHash
		m.update.input.EchoMode = textinput.EchoNormal
		m.update.input.SetValue("")
		m.update.input.Placeholder = "输入 commit 哈希（支持短哈希前缀）"
		m.update.input.Focus()
		return m, textinput.Blink
	default:
		// 列表缓存：短时间内重复进入直接复用，避免无谓消耗 Gitee 额度（匿名额度极低）
		if !m.update.versionsAt.IsZero() && time.Since(m.update.versionsAt) < listCacheTTL {
			m.update.mode = updateVersions
			m.update.versionIx = 0
			m.update.versionPl = listPane{}
			m.setNotice(fmt.Sprintf("复用 %d 个已发布版本（%s 内缓存；刷新请按 r）",
				len(m.update.versions), shortDuration(listCacheTTL)), false)
			return m, nil
		}
		m.update.running = true
		m.update.runningOp = "获取版本列表"
		m.setBusy(m.update.runningOp)
		return m, listVersionsCmd()
	}
}

// runUpdateCmd 后台执行更新包操作
func runUpdateCmd(kind, value string) tea.Cmd {
	return func() tea.Msg {
		cfg := GetUpdatePackConfig()
		if cfg == nil {
			return updateDoneMsg{err: fmt.Errorf("未配置发布通道（local_data/config/release.json 缺少仓库信息）")}
		}
		var err error
		var res updatepack.PullResult
		switch kind {
		case "publish":
			// value 为发布时手动输入的 Gitee 令牌（配置文件不保存密钥）：
			// 传入副本以免污染全局配置
			cfgCopy := *cfg
			cfgCopy.GiteeToken = value
			if perr := updatepack.Publish(cfgCopy, ""); perr != nil {
				return updateDoneMsg{err: perr}
			}
			return updateDoneMsg{}
		case "latest":
			res, err = updatepack.Pull(*withSessionTokenUpdate(cfg), updatepack.PullSpec{Kind: updatepack.PullLatest})
		case "tag":
			res, err = updatepack.Pull(*withSessionTokenUpdate(cfg), updatepack.PullSpec{Kind: updatepack.PullTag, Value: value})
		case "hash":
			res, err = updatepack.Pull(*withSessionTokenUpdate(cfg), updatepack.PullSpec{Kind: updatepack.PullHash, Value: value})
		}
		if err != nil {
			return updateDoneMsg{err: err}
		}
		// 更新包恒含主程序：拉起更新应用器后退出 TUI，
		// 由应用器在本进程释放文件占用后覆盖并重启
		if aerr := updatepack.ApplyProgramUpdate(res, *cfg); aerr != nil {
			return updateDoneMsg{err: aerr}
		}
		// 记录交接说明：控制台退出后打印到真实终端，避免「程序凭空消失」
		self, _ := os.Executable()
		note := &handoverNote{
			PackFile: filepath.Base(res.PackPath),
			Program:  self,
			Root:     updatepack.ProgramRoot(*cfg),
			Files:    res.ProgramFiles,
			LogPath:  updatepack.ApplyLogPath(*cfg),
			Applier:  updatepack.ApplierPath(),
		}
		return updateDoneMsg{restart: true, handover: note}
	}
}

// listVersionsCmd 后台获取版本列表。
//
// 会话令牌会一并带上（配置里有则以配置为准）；若正处于限流冷却期则不请求，
// 直接返回人话提示，避免用户反复刷新把额度耗得更久。
func listVersionsCmd() tea.Cmd {
	return func() tea.Msg {
		if wait := rateLimitRemaining(); wait > 0 {
			return updateVersionsMsg{err: fmt.Errorf("Gitee 接口限流冷却中，约 %s 后可重试", shortDuration(wait))}
		}
		cfg := GetUpdatePackConfig()
		if cfg == nil {
			return updateVersionsMsg{err: fmt.Errorf("未配置发布通道（local_data/config/release.json 缺少仓库信息）")}
		}
		versions, err := updatepack.ListVersions(*withSessionTokenUpdate(cfg))
		if err != nil {
			noteAPIRateLimit(err)
			return updateVersionsMsg{err: err}
		}
		clearRateLimit()
		return updateVersionsMsg{versions: versions}
	}
}

// updateContext 标题栏上下文：仓库与应用目标
func updateContext(s updateState) string {
	cfg := GetUpdatePackConfig()
	if cfg == nil || cfg.GiteeOwner == "" {
		return "未配置仓库"
	}
	ctx := cfg.GiteeOwner + "/" + cfg.GiteeRepo
	target := cfg.TargetDir
	if target == "" || target == "." {
		target = "程序根目录"
	}
	ctx += " · 应用目标 " + target
	switch s.mode {
	case updateVersions:
		ctx += " · 版本列表（r 刷新）"
	case updateInputTag:
		ctx += " · 按 tag 拉取"
	case updateInputHash:
		ctx += " · 按哈希拉取"
	case updateInputToken:
		ctx += " · 发布更新包"
	case updateHandover:
		ctx += " · 已交接更新器"
	}
	return ctx
}

// updateKeys 更新包屏键位（随模式变化）
func updateKeys(s updateState) []keyHint {
	if s.mode == updateHandover {
		return []keyHint{
			kh("Enter/Esc", "立即退出控制台（更新器已在等待）"),
		}
	}
	if s.running {
		return []keyHint{kh("…", "任务进行中，完成前不可操作")}
	}
	switch s.mode {
	case updateInputTag, updateInputHash, updateInputToken:
		return []keyHint{kh("Enter", "确认"), kh("Esc", "返回操作菜单")}
	case updateVersions:
		return []keyHint{
			kh("↑/↓ PgUp/PgDn", "选择版本"),
			kh("Enter", "按该 tag 拉取"),
			kh("r", "刷新列表"),
			kh("Esc/←", "返回操作菜单"),
		}
	}
	return []keyHint{
		kh("↑/↓", "选择操作"),
		kh("Enter", "执行"),
		kh("1-5", "快捷键直达"),
		kh("Esc", "返回菜单"),
	}
}
