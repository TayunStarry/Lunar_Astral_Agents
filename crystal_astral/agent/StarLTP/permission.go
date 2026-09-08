package StarLTP

// ==== LTP9 权限密钥机制 ====
// permissions.key 是被「代码文件哈希加密」的 allow-* 权限清单。
// 授权：取 execute.js 的哈希作解密密钥，把授权清单编码写入 permissions.key。
// 加载：引擎对当前 execute.js 重新哈希并解密，解出 allow-* 后注入对应能力；解码失败/名称非法即拒绝。

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	lunardecoder "LunarSubsystem/LunarDecoder"
)

// permSeparator 授权项之间的分隔符（取自 base64 字符集；allow-* 名不含它）。
const permSeparator = "+"

// permPadding 每条授权项的填充字符（同样取自 base64 字符集）。
const permPadding = "/"

// perPermissionKeyLen 每条授权项的编码长度。
const perPermissionKeyLen = 32

// codeHash compute the哈希 of the plugin's bound code file (execute.js) as 128-bit hex (lunar_decoder key).
func codeHash(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, DefaultMain))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16]), nil
}

// permAllowed 判断权限名是否为 LTP9 支持的合法权限。
func permAllowed(name string) bool {
	return allowedSet[name]
}

// verifyPermissions 读取插件 permissions.key，用代码哈希整体解码 → 按分隔符分割 → 去填充得到 allow-* 集合。
// 解码失败或名称非法 → 视为密钥与代码不对应，拒绝对应权限；开发模式跳过校验并默认授予全部权限。
func (p *plugin) verifyPermissions() map[string]bool {
	if *GeneralConfig.Developer {
		LoggerGeneral.Info(ServiceName, "开发模式启用，插件 %s 默认授予全部 allow-* 权限（跳过权限密钥校验）", p.ID)
		return allGranted()
	}
	granted := map[string]bool{}
	keyStr, err := codeHash(p.Root)
	if err != nil {
		LoggerGeneral.Warn(ServiceName, "插件 %s 代码哈希失败: %v", p.ID, err)
		return granted
	}
	raw, err := os.ReadFile(p.KeyPath)
	if err != nil {
		LoggerGeneral.Warn(ServiceName, "插件 %s 缺少权限密钥文件 %s，按无权限加载（仅保留基础 engine API）", p.ID, p.KeyPath)
		return granted
	}

	decoded, derr := lunardecoder.DecodeFilesWithKeyString([]lunardecoder.FileData{{Name: "perm", Data: raw}}, keyStr)
	if derr != nil || len(decoded) != 1 {
		// 解码失败：代码与密钥不对应或密钥被篡改 → 拒绝全部权限（不退化到明文）
		LoggerGeneral.Warn(ServiceName, "插件 %s 权限密钥解码失败，已拒绝全部权限", p.ID)
		return granted
	}
	payload := string(decoded[0].Data)
	for _, block := range strings.Split(payload, permSeparator) {
		name := strings.TrimSpace(strings.TrimRight(block, permPadding))
		if name == "" {
			continue
		}
		if !permAllowed(name) {
			LoggerGeneral.Warn(ServiceName, "插件 %s 权限密钥解出非法权限名: %q", p.ID, name)
			continue
		}
		granted[name] = true
	}
	if len(granted) == 0 {
		LoggerGeneral.Warn(ServiceName, "插件 %s 权限密钥无法与代码对应或代码被篡改，未授予任何权限", p.ID)
	}
	return granted
}

// allGranted 返回全部 allow-* 权限集合。
func allGranted() map[string]bool {
	set := map[string]bool{}
	for _, n := range AllowPermissionNames {
		set[n] = true
	}
	return set
}

// hasPerm 判断插件是否被授予某权限。
func (p *plugin) hasPerm(name string) bool {
	return p.granted[name]
}