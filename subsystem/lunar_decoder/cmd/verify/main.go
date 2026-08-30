// 本程序使用「月华」目录下的真实图片文件，验证 lunar-decoder 的加密/解密全流程：
//  1. 生成 34 位数字密钥；
//  2. 读取源文件 -> 加密 -> 存档到「缓存」目录（*.enc）；
//  3. 用同一密钥解密「缓存」中的文件 -> 保存到「结果」目录（还原原文件名）；
//  4. 逐文件比对「结果」与原始内容的 SHA-256 是否一致。
//
// 每个步骤都会将过程清晰打印到日志。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	decoder "LunarSubsystem/LunarDecoder"
)

const (
	srcDir    = "d:\\Lunar_Astral_Agents\\subsystem\\lunar_decoder\\月华"
	cacheDir  = "d:\\Lunar_Astral_Agents\\subsystem\\lunar_decoder\\缓存"
	resultDir = "d:\\Lunar_Astral_Agents\\subsystem\\lunar_decoder\\结果"
	keyDigits = 34 // 生成 34 位数字密钥
)

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:16]) // 取前 16 字节避免日志过长
}

// generateKey 生成 digits 位随机数字密钥字符串。
func generateKey(digits int) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, digits)
	first := r.Intn(9) + 1 // 首位非 0，保证位数有效
	b[0] = byte('0' + first)
	for i := 1; i < digits; i++ {
		b[i] = byte('0' + r.Intn(10))
	}
	return string(b)
}

// resetDir 清空并重建目录，保证每次运行结果干净。
func resetDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}

func main() {
	// ---------- 0. 初始化 ----------
	fmt.Println("══════════════════════════════════════════════════")
	fmt.Println("   lunar-decoder 加密/解密全流程验证")
	fmt.Println("══════════════════════════════════════════════════")

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		fmt.Println("[错误] 读取源目录失败:", err)
		os.Exit(1)
	}
	var files []decoder.FileData
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			fmt.Printf("[错误] 读取 %s 失败: %v\n", e.Name(), err)
			continue
		}
		files = append(files, decoder.FileData{Name: e.Name(), Data: data})
	}
	if len(files) == 0 {
		fmt.Println("[错误] 源目录无文件")
		os.Exit(1)
	}
	if err := resetDir(cacheDir); err != nil {
		fmt.Println("[错误] 初始化缓存目录失败:", err)
		os.Exit(1)
	}
	if err := resetDir(resultDir); err != nil {
		fmt.Println("[错误] 初始化结果目录失败:", err)
		os.Exit(1)
	}
	fmt.Printf("\n[0] 源目录: %s （%d 个文件）\n", srcDir, len(files))
	fmt.Printf("    缓存目录: %s\n    结果目录: %s\n", cacheDir, resultDir)

	// ---------- 1. 生成 34 位密钥 ----------
	key := generateKey(keyDigits)
	fmt.Printf("\n[1] 已生成 %d 位密钥: %s\n", len(key), key)
	groups, kgErr := decoder.KeyGroupsFromString(key)
	if kgErr != nil {
		fmt.Println("[错误] 密钥处理失败:", kgErr)
		os.Exit(1)
	}
	fmt.Printf("    密钥标准化合并后拆分为 8 组操作:\n")
	for i, g := range groups {
		fmt.Printf("      组[%d] 类型=%d 次数=%d\n", i, g.Op, g.Count)
	}

	// ---------- 2. 加密并存档到「缓存」 ----------
	var encBytes int64
	encStart := time.Now()
	for _, f := range files {
		enc, err := decoder.EncodeFilesWithKeyString([]decoder.FileData{f}, key)
		if err != nil {
			fmt.Printf("[错误] 加密 %s 失败: %v\n", f.Name, err)
			os.Exit(1)
		}
		dst := filepath.Join(cacheDir, f.Name+".enc")
		if err := os.WriteFile(dst, enc[0].Data, 0o644); err != nil {
			fmt.Printf("[错误] 写入缓存失败 %s: %v\n", dst, err)
			os.Exit(1)
		}
		encBytes += int64(len(enc[0].Data))
		fmt.Printf("[2] 加密 %-32s %7d 字节 -> %7d 字节  =>  %s\n",
			f.Name, len(f.Data), len(enc[0].Data), dst)
	}
	encDur := time.Since(encStart)
	fmt.Printf("    共加密 %d 个文件，已存档到「缓存」目录（耗时 %v）\n", len(files), encDur)

	// ---------- 3. 解密「缓存」并存档到「结果」 ----------
	var decoded []decoder.FileData
	decStart := time.Now()
	cacheEntries, _ := os.ReadDir(cacheDir)
	for _, e := range cacheEntries {
		data, err := os.ReadFile(filepath.Join(cacheDir, e.Name()))
		if err != nil {
			fmt.Printf("[错误] 读取缓存失败 %s: %v\n", e.Name(), err)
			os.Exit(1)
		}
		dec, err := decoder.DecodeFilesWithKeyString([]decoder.FileData{{Name: e.Name(), Data: data}}, key)
		if err != nil {
			fmt.Printf("[错误] 解密 %s 失败: %v\n", e.Name(), err)
			os.Exit(1)
		}
		// 去掉 .enc 后缀还原原文件名。
		origName := e.Name()
		if filepath.Ext(origName) == ".enc" {
			origName = origName[:len(origName)-len(".enc")]
		}
		dst := filepath.Join(resultDir, origName)
		if err := os.WriteFile(dst, dec[0].Data, 0o644); err != nil {
			fmt.Printf("[错误] 写入结果失败 %s: %v\n", dst, err)
			os.Exit(1)
		}
		decoded = append(decoded, decoder.FileData{Name: origName, Data: dec[0].Data})
		fmt.Printf("[3] 解密 %-32s %7d 字节 -> %7d 字节  =>  %s\n",
			e.Name(), len(data), len(dec[0].Data), dst)

	}
	decDur := time.Since(decStart)
	fmt.Printf("    共解密 %d 个文件，已存档到「结果」目录（耗时 %v）\n", len(decoded), decDur)

	// ---------- 4. 比对「结果」与原始文件内容 ----------
	fmt.Println("\n[4] 结果比对（SHA-256 前 16 字节）：")
	pass, fail := 0, 0
	for i, f := range files {
		res := decoded[i]
		want := sha256Hex(f.Data)
		got := sha256Hex(res.Data)
		ok := res.Name == f.Name && string(want) == string(got)
		mark := "[一致]"
		if !ok {
			mark = "[不一致]"
			fail++
		} else {
			pass++
		}
		fmt.Printf("    %s %-32s 原始%s 还原%s  %s\n",
			mark, f.Name, want, got, res.Name)
	}

	// ---------- 汇总 ----------
	var origBytes int64
	for _, f := range files {
		origBytes += int64(len(f.Data))
	}
	fmt.Println("\n══════════════════════════════════════════════════")
	fmt.Printf("原始总量: %d 字节 | 加密总量: %d 字节 | 存储膨胀率: %.2f%%\n",
		origBytes, encBytes, float64(encBytes-origBytes)/float64(origBytes)*100)
	fmt.Printf("密钥: %s（%d 位）\n", key, len(key))
	fmt.Printf("比对结果: 通过 %d / %d\n", pass, len(files))
	if fail == 0 {
		fmt.Println("结论: 全部一致 ✅ 加密 -> 存档 -> 解密 -> 还原 流程完整可用")
	} else {
		fmt.Printf("结论: %d 个文件不一致 ❌\n", fail)
		os.Exit(1)
	}
}
