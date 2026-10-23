package StarLTP

// ==== LTP9 授权签发（控制台「LTP9 授权签发」视图的引擎侧实现） ====
//
// 与 permission.go 的解码侧成对：签发用 execute.js 的 sha16 作密钥，把 allow-* 清单整体
// 加密写入 permissions.key；加载时引擎对同一 execute.js 重新哈希并解密。签发规则与独立
// 工具 subsystem/ltp9_keygen 的 Web 实现完全一致，但**授权范围由脚本现有引用推断**——
// 这是本控制台版本相对「手工勾选权限」的核心差异。
//
// 为什么可以按引用推断：LTP9 的权限按「顶层全局对象」分布，见 binder.go 的注入表：
//   allow-signal→signal · allow-file→file/image/video · allow-memory→memory
//   allow-database→database · allow-certificate→encoding/hash · allow-call→callFunction
//   allow-agent→agent · allow-network→http/network/fetch/WebSocket
// 脚本引用了哪个根对象，就为它签发哪项权限。预留权限（allow-send / allow-socket，
// Flash 尚未挂载能力）永不自动签发。

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	lunardecoder "LunarSubsystem/LunarDecoder"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// PackageAuth 单包的授权现状（控制台列表与签发回执共用同一结构）
type PackageAuth struct {
	Root      string              `json:"root"`       // 包目录绝对路径
	HasScript bool                `json:"has_script"` // execute.js 是否存在（不存在即不可签发）
	HasKey    bool                `json:"has_key"`    // permissions.key 是否存在
	Signed    bool                `json:"signed"`     // 现有密钥能否用当前脚本哈希解开（与运行时判据一致）
	Granted   []string            `json:"granted"`    // 现有密钥解出的权限（未签发时为空）
	Required  []string            `json:"required"`   // 脚本现有引用推断出的权限（按权限全集顺序）
	Reasons   map[string][]string `json:"reasons"`    // 权限 → 命中的调用点（控制台确认页呈现依据）
	Hash      string              `json:"hash"`       // execute.js 的 sha16（展示与比对用）
}

// codeHashFromData 计算脚本文件的 128 位哈希（sha256 前 16 字节的 hex，作解码器密钥）。
// 规则与 permission.go 的 codeHash / 独立工具 keygen.go 完全一致。
func codeHashFromData(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}

// ==== 脚本权限推断 ====

// rootObjectPermissions 根对象 → 权限（与 binder.go 的沙箱注入表一一对应）
var rootObjectPermissions = []struct {
	name string
	perm string
}{
	{"signal", "allow-signal"},
	{"file", "allow-file"},
	{"image", "allow-file"},
	{"video", "allow-file"},
	{"memory", "allow-memory"},
	{"database", "allow-database"},
	{"encoding", "allow-certificate"},
	{"hash", "allow-certificate"},
	{"agent", "allow-agent"},
	{"http", "allow-network"},
	{"network", "allow-network"},
}

// globalCallPermissions 全局调用形态 → 权限（label 为控制台展示的命中点写法）
var globalCallPermissions = []struct {
	frag  string
	label string
	perm  string
}{
	{`callFunction\s*\(`, "callFunction", "allow-call"},
	{`fetch\s*\(`, "fetch", "allow-network"},
	{`WebSocket\s*\(`, "WebSocket", "allow-network"},
}

// dualPermissionRefs 运行时另需 allow-network 的 API（无条件需要网络，签发时一并授予；
// 见 binder.go 的契约注释与 api_media.go 的运行时检查）
var dualPermissionRefs = []struct {
	frag  string
	label string
}{
	{`image\s*\.\s*download\s*\(`, "image.download"},
	{`agent\s*\.\s*search\s*\(`, "agent.search"},
}

// reCacheMu / reCache 正则缓存（推断在小列表上重复调用，避免每次编译）
var (
	reCacheMu sync.Mutex
	reCache   = map[string]*regexp.Regexp{}
)

// cachedRe 编译并缓存正则（正则非法时 panic：模式由本文件内的常量拼出，属编程错误）
func cachedRe(pattern string) *regexp.Regexp {
	reCacheMu.Lock()
	defer reCacheMu.Unlock()
	if re, ok := reCache[pattern]; ok {
		return re
	}
	re := regexp.MustCompile(pattern)
	reCache[pattern] = re
	return re
}

// rootRefRe 根对象调用形态：`signal.all` 这类「根对象 + 成员访问」。
// 前缀要求「非标识符字符且非 . 与 $」，避免 x.file / $file. / myFile 误命中。
func rootRefRe(name string) *regexp.Regexp {
	return cachedRe(`(?:^|[^\w.$])` + name + `\s*\.`)
}

// globalCallRe 全局调用形态（同样加词边界前缀）
func globalCallRe(frag string) *regexp.Regexp {
	return cachedRe(`(?:^|[^\w.$])` + frag)
}

// RequiredPermissions 从脚本源码推断其引用的 allow-* 权限（按权限全集顺序返回，空表示
// 只用常驻能力）。判据只认「调用根对象」（`file.read(...)`），不认裸标识符——与权限按
// 根对象分布的设计一致，也避免把同名局部变量当成能力调用。
//
// 取舍偏保守（宁可多签、绝不漏签）：注释与同行闭合的引号字符串会被屏蔽，模板字符串内容
// 不屏蔽。少签会让插件在运行时被 ReferenceError 挡住，多签只会让控制台多列一项，
// 且签发前必须由用户确认。
func RequiredPermissions(script []byte) []string {
	perms, _ := analyzePermissionRefs(script)
	return perms
}

// analyzePermissionRefs 推断权限并同时给出命中依据（权限 → 命中的调用点），
// 供控制台在签发确认页把「为什么给这项权限」摆给用户看。
func analyzePermissionRefs(script []byte) ([]string, map[string][]string) {
	code := maskJSNonCode(script)
	granted := map[string]bool{}
	reasons := map[string][]string{}
	add := func(perm, reason string) {
		granted[perm] = true
		for _, seen := range reasons[perm] {
			if seen == reason {
				return
			}
		}
		reasons[perm] = append(reasons[perm], reason)
	}

	for _, r := range rootObjectPermissions {
		if rootRefRe(r.name).MatchString(code) {
			add(r.perm, r.name+".")
		}
	}
	for _, c := range globalCallPermissions {
		if globalCallRe(c.frag).MatchString(code) {
			add(c.perm, c.label)
		}
	}
	for _, d := range dualPermissionRefs {
		if globalCallRe(d.frag).MatchString(code) {
			add("allow-network", d.label)
		}
	}

	out := make([]string, 0, len(granted))
	for _, name := range AllowPermissionNames {
		if granted[name] {
			out = append(out, name)
		}
	}
	return out, reasons
}

// maskJSNonCode 把注释与「同行闭合的引号字符串」替换为同长空格，保留换行。
//
// 只做屏蔽、不做语法分析：未闭合的引号（含正则字面量里的引号）不进入字符串态，
// 因此不会因一个引号把后面整段代码吞掉——那会造成漏签，比多签危险得多。
func maskJSNonCode(src []byte) string {
	out := make([]byte, len(src))
	copy(out, src)

	blank := func(from, to int) {
		for i := from; i < to && i < len(out); i++ {
			if out[i] != '\n' && out[i] != '\r' {
				out[i] = ' '
			}
		}
	}
	// 引号字符串：仅屏蔽「同一行内闭合」的片段
	quoteSpan := func(i int, quote byte) int {
		for j := i + 1; j < len(src); j++ {
			switch src[j] {
			case '\\':
				j++ // 转义字符：跳过下一个字节
			case '\n':
				return -1 // 本行未闭合：视为非字符串（正则字面量等），原样保留
			case quote:
				return j + 1
			}
		}
		return -1
	}

	for i := 0; i < len(src); i++ {
		switch {
		case src[i] == '/' && i+1 < len(src) && src[i+1] == '/': // 行注释
			end := i
			for end < len(src) && src[end] != '\n' {
				end++
			}
			blank(i, end)
			i = end - 1
		case src[i] == '/' && i+1 < len(src) && src[i+1] == '*': // 块注释
			end := i + 2
			for end+1 < len(src) && !(src[end] == '*' && src[end+1] == '/') {
				end++
			}
			if end+1 < len(src) {
				end += 2
			} else {
				end = len(src)
			}
			blank(i, end)
			i = end - 1
		case src[i] == '\'' || src[i] == '"': // 同行闭合的字符串
			if end := quoteSpan(i, src[i]); end > 0 {
				blank(i, end)
				i = end - 1
			}
		}
	}
	return string(out)
}

// ==== 现状读取与签发 ====

// InspectPackageAuth 读取包的授权现状，不修改任何文件。
// 无入口脚本时返回 HasScript=false（调用方据此呈现「不可签发」）；
// 密钥与当前脚本不对应时 Signed=false（调用方据此呈现「需重签」）。
func InspectPackageAuth(root string) (PackageAuth, error) {
	auth := PackageAuth{Root: root}
	data, err := os.ReadFile(filepath.Join(root, DefaultMain))
	if err != nil {
		return auth, nil
	}
	auth.HasScript = true
	auth.Hash = codeHashFromData(data)
	auth.Required, auth.Reasons = analyzePermissionRefs(data)

	raw, err := os.ReadFile(filepath.Join(root, KeyFileName))
	if err != nil {
		return auth, nil
	}
	auth.HasKey = true
	granted, ok, illegal := grantedFromKeyData(raw, auth.Hash)
	if len(illegal) > 0 {
		logger.Warn(ServiceName, "包 %s 权限密钥含非法权限名（已按解码规则丢弃）: %s",
			filepath.Base(root), strings.Join(illegal, "、"))
	}
	if !ok {
		return auth, nil
	}
	auth.Signed = true
	auth.Granted = orderPermissions(granted)
	return auth, nil
}

// SignPackageAuth 依据脚本现有引用签发授权（覆盖写入 permissions.key），返回签发后的现状。
// 脚本未引用任何 allow-* 时不做任何写入（只使用常驻能力，无需签发），由调用方提示。
func SignPackageAuth(root string) (PackageAuth, error) {
	auth, err := InspectPackageAuth(root)
	if err != nil {
		return auth, err
	}
	if !auth.HasScript {
		return auth, fmt.Errorf("包内没有 %s，无法签发", DefaultMain)
	}
	if len(auth.Required) == 0 {
		return auth, nil
	}

	payload := strings.Join(buildPermissionKeys(auth.Required), permSeparator)
	outs, err := lunardecoder.EncodeFilesWithKeyString(
		[]lunardecoder.FileData{{Name: "perm", Data: []byte(payload)}}, auth.Hash)
	if err != nil {
		return auth, fmt.Errorf("加密权限清单失败: %v", err)
	}
	if len(outs) != 1 {
		return auth, fmt.Errorf("加密权限清单失败: 期望 1 段密文，实得 %d", len(outs))
	}
	keyPath := filepath.Join(root, KeyFileName)
	if err := os.WriteFile(keyPath, outs[0].Data, 0644); err != nil {
		return auth, fmt.Errorf("写入 %s 失败: %v", KeyFileName, err)
	}
	logger.Info(ServiceName, "已为包 %s 签发权限密钥（%d 项，代码哈希 %s）: %s",
		filepath.Base(root), len(auth.Required), auth.Hash[:8], strings.Join(auth.Required, "、"))

	return InspectPackageAuth(root)
}

// buildPermissionKeys 为每项权限生成一条固定长度密钥串：「权限名 + '/' 填充」，
// 与独立工具 keygen.go 的规则严格一致（每条独立对应一项权限，整体拼接后再加密）。
func buildPermissionKeys(perms []string) []string {
	keys := make([]string, 0, len(perms))
	for _, perm := range perms {
		block := perm
		if len(block) >= perPermissionKeyLen {
			block = block[:perPermissionKeyLen]
		} else {
			block += strings.Repeat(permPadding, perPermissionKeyLen-len(block))
		}
		keys = append(keys, block)
	}
	return keys
}

// orderPermissions 把权限集合按全集顺序排列（展示与比对都依赖稳定顺序）
func orderPermissions(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for _, name := range AllowPermissionNames {
		if set[name] {
			out = append(out, name)
		}
	}
	return out
}
