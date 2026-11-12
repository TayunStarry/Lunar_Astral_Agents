package main

// ==== 更新器 TUI 调色板与样式 ====
//
// 与主控制台同源：品牌紫承担标题与选中态，青绿承担数值与成功，橙/红承担警告与错误，
// 灰阶承担结构线与次要信息。全部使用 256 色索引，conhost / Windows Terminal 表现一致。

import "github.com/charmbracelet/lipgloss"

var (
	colAccent     = lipgloss.Color("213")
	colAccentSoft = lipgloss.Color("219")
	colText       = lipgloss.Color("252")
	colValue      = lipgloss.Color("86")
	colDim        = lipgloss.Color("245")
	colFaint      = lipgloss.Color("240")
	colRule       = lipgloss.Color("238")
	colOK         = lipgloss.Color("82")
	colWarn       = lipgloss.Color("214")
	colErr        = lipgloss.Color("204")
	colSelBg      = lipgloss.Color("237")
	colSelFg      = lipgloss.Color("231")
	colSelDim     = lipgloss.Color("250")
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	crumbStyle   = lipgloss.NewStyle().Foreground(colDim)
	labelStyle   = lipgloss.NewStyle().Foreground(colText)
	valueStyle   = lipgloss.NewStyle().Bold(true).Foreground(colValue)
	dimStyle     = lipgloss.NewStyle().Foreground(colDim)
	faintStyle   = lipgloss.NewStyle().Foreground(colFaint)
	selectStyle  = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	okStyle      = lipgloss.NewStyle().Bold(true).Foreground(colOK)
	warnStyle    = lipgloss.NewStyle().Foreground(colWarn)
	errStyle     = lipgloss.NewStyle().Bold(true).Foreground(colErr)
	ruleStyle    = lipgloss.NewStyle().Foreground(colRule)
	keyStyle     = lipgloss.NewStyle().Bold(true).Foreground(colAccentSoft)
	hintStyle    = lipgloss.NewStyle().Foreground(colDim)
	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(colValue)
	selRowStyle  = lipgloss.NewStyle().Background(colSelBg).Foreground(colSelFg)
	selDimStyle  = lipgloss.NewStyle().Background(colSelBg).Foreground(colSelDim)

	// 侧栏与列表光标：与主控制台 engine/console 的 nav* / cursor 样式一一对应
	cursorStyle   = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	navGroupStyle = lipgloss.NewStyle().Bold(true).Foreground(colDim)
	navItemStyle  = lipgloss.NewStyle().Foreground(colText)
	navKeyStyle   = lipgloss.NewStyle().Foreground(colFaint)
)
