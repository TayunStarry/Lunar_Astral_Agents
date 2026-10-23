package StarLTP

// ==== LTP9 权限密钥机制 ====
// permissions.key 是被「代码文件哈希加密」的 allow-* 权限清单。
// 授权：取 execute.js 的哈希作解密密钥，把授权清单编码写入 permissions.key。
// 加载：引擎对当前 execute.js 重新哈希并解密，解出 allow-* 后注入对应能力；解码失败/名称非法即拒绝。

import (
	"os"
	"path/filepath"
	"strings"

	lunardecoder "LunarSubsystem/LunarDecoder"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// permSeparator 授权项之间的分隔符（取自 base64 字符集；allow-* 名不含它）。
const permSeparator = "+"

// permPadding 每条授权项的填充字符（同样取自 base64 字符集）。
const permPadding = "/"

// perPermissionKeyLen 每条授权项的编码长度。
const perPermissionKeyLen = 32

// codeHash 计算插件绑定代码文件（execute.js）的哈希，作 128-bit hex（lunar_decoder 密钥）。
func codeHash(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, DefaultMain))
	if err != nil {
		return "", err
	}
	return codeHashFromData(data), nil
}

// grantedFromKeyData 用代码哈希解开密钥密文，得到 allow-* 权限集合。
//
// 解码与签发的**唯一实现**：加载校验（verifyPermissions）与控制台签发（permission_sign.go
// 的 InspectPackageAuth）共用它，避免两侧各自写一份「分割 + 去填充 + 校验权限名」的规则。
// 返回值：
//   - granted：解出的合法权限集合；
//   - ok：整体解码是否成功（false = 密钥与代码不对应 / 被篡改，调用方应拒绝全部权限）；
//   - illegal：其中非法权限名（按运行时既定语义丢弃，只作留痕）。
func grantedFromKeyData(raw []byte, hashKey string) (map[string]bool, bool, []string) {
	granted := map[string]bool{}
	decoded, derr := lunardecoder.DecodeFilesWithKeyString([]lunardecoder.FileData{{Name: "perm", Data: raw}}, hashKey)
	if derr != nil || len(decoded) != 1 {
		return granted, false, nil
	}
	var illegal []string
	for _, block := range strings.Split(string(decoded[0].Data), permSeparator) {
		name := strings.TrimSpace(strings.TrimRight(block, permPadding))
		if name == "" {
			continue
		}
		if !permAllowed(name) {
			illegal = append(illegal, name)
			continue
		}
		granted[name] = true
	}
	return granted, true, illegal
}

// permAllowed 判断权限名是否为 LTP9 支持的合法权限。
func permAllowed(name string) bool {
	return allowedSet[name]
}

// verifyPermissions 读取插件 permissions.key，用代码哈希整体解码 → 按分隔符分割 → 去填充得到 allow-* 集合。
// 解码失败或名称非法 → 视为密钥与代码不对应，拒绝对应权限；开发模式跳过校验并默认授予全部权限。
func (p *plugin) verifyPermissions() map[string]bool {
	if *config.Developer {
		logger.Info(ServiceName, "开发模式启用，插件 %s 默认授予全部 allow-* 权限（跳过权限密钥校验）", p.ID)
		return allGranted()
	}
	granted := map[string]bool{}
	keyStr, err := codeHash(p.Root)
	if err != nil {
		logger.Warn(ServiceName, "插件 %s 代码哈希失败: %v", p.ID, err)
		return granted
	}
	raw, err := os.ReadFile(p.KeyPath)
	if err != nil {
		logger.Warn(ServiceName, "插件 %s 缺少权限密钥文件 %s，按无权限加载（仅保留基础 engine API）", p.ID, p.KeyPath)
		return granted
	}

	declared, ok, illegal := grantedFromKeyData(raw, keyStr)
	for _, name := range illegal {
		logger.Warn(ServiceName, "插件 %s 权限密钥解出非法权限名: %q", p.ID, name)
	}
	if !ok {
		// 解码失败：代码与密钥不对应或密钥被篡改 → 拒绝全部权限（不退化到明文）
		logger.Warn(ServiceName, "插件 %s 权限密钥解码失败，已拒绝全部权限", p.ID)
		return granted
	}
	for name := range declared {
		granted[name] = true
	}
	if len(granted) == 0 {
		logger.Warn(ServiceName, "插件 %s 权限密钥无法与代码对应或代码被篡改，未授予任何权限", p.ID)
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
