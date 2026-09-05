package main

// ==== 密钥生成核心逻辑（与引擎 agent/YaraLTP/permission.go 规则严格一致） ====

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	lunardecoder "LunarSubsystem/LunarDecoder"
)

// scriptFile 参与哈希的一个脚本文件。
type scriptFile struct {
	Name string
	Data []byte
}

// scriptsHashFromFiles 拼接脚本并计算 128 位截断哈希（hex 32 字符，作为 lunar_decoder 密钥）。
// 拼接顺序：按 Name 字典序，与引擎端一致。
func scriptsHashFromFiles(files []scriptFile) string {
	sorted := make([]scriptFile, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	var buf bytes.Buffer
	for _, f := range sorted {
		buf.Write(f.Data)
	}
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:16])
}

// readScriptsFromDir 读取目录下全部 .js 脚本（跳过 data/ 目录，按相对路径排序）。
func readScriptsFromDir(dir string) ([]scriptFile, error) {
	var paths []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return nil
		}
		if info.IsDir() {
			if strings.EqualFold(info.Name(), "data") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(strings.ToLower(info.Name()), ".js") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	files := make([]scriptFile, 0, len(paths))
	for _, p := range paths {
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil, rerr
		}
		files = append(files, scriptFile{Name: p, Data: b})
	}
	return files, nil
}

// buildPermissionKeys 为每个权限生成一条 32 字符密钥字符串：
// 「权限名 + '/' 填充」，每条独立对应一个权限（10 个权限 → 10 条 32 字符密钥字符串）。
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