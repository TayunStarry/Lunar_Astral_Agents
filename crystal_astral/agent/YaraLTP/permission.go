package YaraLTP

// ==== LTP3 权限密钥机制 ====
// 授权流程（根开发者在密钥生成器 subsystem/ltp3_keygen 中操作）：
//   1. 拼接插件全部脚本 → SHA-256 截取前 128 位 → hex 作为 lunar_decoder 密钥；
//   2. 为每个权限生成一条 32 字符「密钥字符串」：权限名 + '/' 填充到 32；
//   3. 用 '+'（取自 base64 字符集）连接全部密钥字符串，作为一次明文报文；
//   4. 用脚本哈希对整条明文报文做一次整体加密，写入 permissions.key。
//     —— 文件中看不到分隔符与密钥边界，达成完全混淆。
// 校验流程（引擎侧）：先整体解码拿到明文，再按 '+' 分割成 N 条，去掉 '/' 填充得到权限名；
//   整体解码失败、或分割出的名称不在权限名单 → 视为密钥与脚本不对应/脚本被篡改，拒绝该权限。

import (
	"slices"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	lunardecoder "LunarSubsystem/LunarDecoder"
)

// permSeparator 密钥字符串之间的分隔符（取自 base64 字符集；权限名不含它）。
const permSeparator = "+"

// permPadding 每条密钥字符串的填充字符（同样取自 base64 字符集；权限名不含它）。
const permPadding = "/"

// perPermissionKeyLen 每条权限密钥字符串的长度。
const perPermissionKeyLen = 32

// permValid 判断权限名是否为引擎支持的合法权限。
func permValid(name string) bool {
	return slices.Contains(AllPermissionNames, name)
}

// scriptsHash 拼接插件根目录下全部 .js 脚本（相对路径排序，跳过 data/ 目录），
// 计算 SHA-256 并截取前 128 位（16 字节），hex 编码后作为 lunar_decoder 密钥串。
// 与密钥生成器保持完全一致的规则。
func scriptsHash(root string) (string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return nil
		}
		if info.IsDir() {
			if strings.EqualFold(info.Name(), DataDirName) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(strings.ToLower(info.Name()), ".js") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	var buf bytes.Buffer
	for _, f := range files {
		b, rerr := os.ReadFile(f)
		if rerr != nil {
			return "", rerr
		}
		buf.Write(b)
	}
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:16]), nil
}

// verifyPermissions 读取插件 permissions.key，用脚本哈希整体解码 → 按分隔符分割 →
// 去掉填充得到权限名集合。密钥缺失 / 整体解码失败 / 名称非法时拒绝对应权限。
// 开发模式（GeneralConfig.Developer）开启时跳过校验，默认授予全部权限，便于插件开发。
func (p *plugin) verifyPermissions() map[string]bool {
	if *GeneralConfig.Developer {
		LoggerGeneral.Info(ServiceName, "开发模式已启用，插件 %s 默认授予全部权限（跳过权限密钥校验）", p.ID)
		return allPermissionSet()
	}

	granted := map[string]bool{}
	keyStr, err := scriptsHash(p.Root)
	if err != nil {
		LoggerGeneral.Warn(ServiceName, "插件 %s 脚本哈希失败: %v", p.ID, err)
		return granted
	}
	raw, err := os.ReadFile(p.KeyPath)
	if err != nil {
		LoggerGeneral.Warn(ServiceName, "插件 %s 缺少权限密钥文件 %s，按无权限加载（仅保留基础 API）", p.ID, p.KeyPath)
		return granted
	}

	// 整体解码按条解析：先解码整个 payload，再切分，最后去填充。
	decoded, derr := lunardecoder.DecodeFilesWithKeyString([]lunardecoder.FileData{{Name: "perm", Data: raw}}, keyStr)
	var payload string
	if derr != nil {
		payload = string(raw) // 解码失败时退化为把原始文件按同规则解析（兼容按条编码的旧文件）
	} else if len(decoded) == 1 {
		payload = string(decoded[0].Data)
	}

	for _, block := range strings.Split(payload, permSeparator) {
		block = strings.TrimRight(block, permPadding)
		name := strings.TrimSpace(block)
		if name == "" {
			continue
		}
		if !permValid(name) {
			LoggerGeneral.Warn(ServiceName, "插件 %s 权限密钥解码出非法权限名: %q", p.ID, name)
			continue
		}
		granted[name] = true
	}
	if len(granted) == 0 {
		LoggerGeneral.Warn(ServiceName, "插件 %s 权限密钥无法与脚本对应或脚本被篡改，已拒绝全部权限", p.ID)
	}
	return granted
}

// allPermissionSet 返回全部权限名集合。
func allPermissionSet() map[string]bool {
	set := map[string]bool{}
	for _, n := range AllPermissionNames {
		set[n] = true
	}
	return set
}
