package main

// ==== 密钥生成核心逻辑（与引擎 crystal_astral/agent/LTP9/permission.go 规则严格一致） ====

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	lunardecoder "LunarSubsystem/LunarDecoder"
)

// codeHashFromData 计算插件绑定代码文件（execute.js 单一文件）的 128 位哈希（hex 32 字符，作为 lunar_decoder 密钥）。
// 规则与引擎 LTP9 permission.go 的 codeHash 完全一致：sha256(文件字节)[:16]。
func codeHashFromData(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}

// readExecuteJSFromDir 从插件包目录读取打包后的 execute.js（单一自包含入口，即被哈希绑定文件）。
func readExecuteJSFromDir(dir string) (string, []byte, error) {
	path := filepath.Join(dir, defaultMain)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("读取 %s 失败: %v", defaultMain, err)
	}
	return path, data, nil
}

// buildPermissionKeys 为每个权限生成一条 32 字符密钥字符串：
// 「权限名 + '/' 填充」，每条独立对应一个权限（8 个权限 → 8 条 32 字符密钥字符串）。
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

// encryptPermissionKeys 用脚本哈希加密整条明文报文（N 条密钥字符串以 '+' 连接）：
// 文件仅含一段密文，分隔符与密钥边界在密文中不可见，需整体解码后才能分割。
func encryptPermissionKeys(keys []string, hashKey string) (string, error) {
	payload := strings.Join(keys, permSeparator)
	outs, err := lunardecoder.EncodeFilesWithKeyString([]lunardecoder.FileData{{Name: "perm", Data: []byte(payload)}}, hashKey)
	if err != nil {
		return "", err
	}
	return string(outs[0].Data), nil
}

// validatePerms 校验权限名集合全部合法。
func validatePerms(perms []string) error {
	for _, p := range perms {
		if !permIndex[p] {
			return fmt.Errorf("未知权限名: %s", p)
		}
	}
	return nil
}