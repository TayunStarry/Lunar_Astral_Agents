// 验证 lunar-decoder 加解密全流程：生成密钥 -> 加密到「缓存」-> 解密到「结果」-> SHA-256 比对。
// 源文件优先用「月华」目录真实图片，缺失时生成合成样本。
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
	keyDigits = 34
)

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:16])
}

// encName 由源文件名派生加密文件名（SHA-256 前 16 位十六进制 + ".lunar"）。
func encName(srcName string) string {
	sum := sha256.Sum256([]byte(srcName))
	return hex.EncodeToString(sum[:8]) + ".lunar"
}

// generateKey 生成 digits 位随机数字密钥。
func generateKey(digits int) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, digits)
	b[0] = byte('0' + r.Intn(9) + 1)
	for i := 1; i < digits; i++ {
		b[i] = byte('0' + r.Intn(10))
	}
	return string(b)
}

// resetDir 清空并重建目录。
func resetDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}

// loadSourceFiles 读取源目录文件，缺失/为空时生成合成样本。
func loadSourceFiles() ([]decoder.FileData, string, error) {
	entries, err := os.ReadDir(srcDir)
	usingSamples := false
	if err != nil || len(entries) == 0 {
		fmt.Println("[0] 源目录缺失或为空，自动生成合成样本文件进行验证")
		usingSamples = true
		entries = nil
	}

	var files []decoder.FileData
	if !usingSamples {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
			if err != nil {
				fmt.Printf("    跳过读取失败文件 %s: %v\n", e.Name(), err)
				continue
			}
			files = append(files, decoder.FileData{Name: e.Name(), Data: data})
		}
	}
	if len(files) == 0 {
		if !usingSamples {
			fmt.Println("[0] 源目录无文件，自动生成合成样本文件进行验证")
		}
		r := rand.New(rand.NewSource(20260830))
		samples := []decoder.FileData{
			{Name: "sample_text.txt", Data: []byte("星月智能 · lunar-decoder 上下文窗口滑移加密验证\n这是样本文本文件，用于验证编解码往返一致性。")},
			{Name: "sample_binary.bin", Data: makeRandomBytes(r, 2048)},
			{Name: "sample_runs.bin", Data: bytesRepeat(0x00, 400)},
		}
		files = samples
	}
	return files, "", nil
}

func makeRandomBytes(r *rand.Rand, n int) []byte {
	b := make([]byte, n)
	_, _ = r.Read(b)
	return b
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func main() {
	fmt.Println("══════════════════════════════════════════════════")
	fmt.Println("   lunar-decoder 加密/解密全流程验证")
	fmt.Println("══════════════════════════════════════════════════")

	files, _, err := loadSourceFiles()
	if err != nil || len(files) == 0 {
		fmt.Println("[错误] 无法获取源文件")
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
	fmt.Printf("\n[0] 源文件: %d 个\n", len(files))
	fmt.Printf("    缓存目录: %s\n    结果目录: %s\n", cacheDir, resultDir)

	// 1. 生成密钥并展示各轮派生结果。
	key := generateKey(keyDigits)
	fmt.Printf("\n[1] 已生成 %d 位密钥: %s\n", len(key), key)
	rounds, kgErr := decoder.KeyRoundsFromString(key)
	if kgErr != nil {
		fmt.Println("[错误] 密钥处理失败:", kgErr)
		os.Exit(1)
	}
	fmt.Printf("    密钥拆分为 %d 轮：\n", len(rounds))
	for ri, r := range rounds {
		fmt.Printf("      轮[%d] 密钥=%s\n", ri, r.Key)
		for i, g := range r.Groups {
			fmt.Printf("        组[%d] 类型=%d 次数=%d\n", i, g.Op, g.Count)
		}
		spec := decoder.SlideSpecFromGroups(r.Groups)
		dirName := map[decoder.SlideDirection]string{
			decoder.SlideNone:     "不滑移",
			decoder.SlideBackward: "向后滑移",
			decoder.SlideForward:  "向前滑移",
		}[spec.Direction]
		fmt.Printf("        滑移: 偶数组 %d / 奇数组 %d -> %s\n",
			spec.EvenCount, spec.OddCount, dirName)
	}

	// 2. 加密并存档到「缓存」。
	var encBytes int64
	encStart := time.Now()
	for _, f := range files {
		enc, err := decoder.EncodeFilesWithKeyString([]decoder.FileData{f}, key)
		if err != nil {
			fmt.Printf("[错误] 加密 %s 失败: %v\n", f.Name, err)
			os.Exit(1)
		}
		encFile := encName(f.Name)
		dst := filepath.Join(cacheDir, encFile)
		if err := os.WriteFile(dst, enc[0].Data, 0o644); err != nil {
			fmt.Printf("[错误] 写入缓存失败 %s: %v\n", dst, err)
			os.Exit(1)
		}
		encBytes += int64(len(enc[0].Data))
		fmt.Printf("[2] 加密 %-32s %7d 字节 -> %7d 字节  =>  %s\n",
			f.Name, len(f.Data), len(enc[0].Data), encFile)
	}
	encDur := time.Since(encStart)
	fmt.Printf("    共加密 %d 个文件，已存档到「缓存」目录（耗时 %v）\n", len(files), encDur)

	// 3. 解密「缓存」并存档到「结果」。
	var decoded []decoder.FileData
	decStart := time.Now()
	for _, f := range files {
		encFile := encName(f.Name)
		data, err := os.ReadFile(filepath.Join(cacheDir, encFile))
		if err != nil {
			fmt.Printf("[错误] 读取缓存失败 %s: %v\n", encFile, err)
			os.Exit(1)
		}
		dec, err := decoder.DecodeFilesWithKeyString([]decoder.FileData{{Name: f.Name, Data: data}}, key)
		if err != nil {
			fmt.Printf("[错误] 解密 %s 失败: %v\n", encFile, err)
			os.Exit(1)
		}
		dst := filepath.Join(resultDir, f.Name)
		if err := os.WriteFile(dst, dec[0].Data, 0o644); err != nil {
			fmt.Printf("[错误] 写入结果失败 %s: %v\n", dst, err)
			os.Exit(1)
		}
		decoded = append(decoded, decoder.FileData{Name: f.Name, Data: dec[0].Data})
		fmt.Printf("[3] 解密 %-32s %7d 字节 -> %7d 字节  =>  %s\n",
			encFile, len(data), len(dec[0].Data), f.Name)
	}
	decDur := time.Since(decStart)
	fmt.Printf("    共解密 %d 个文件，已存档到「结果」目录（耗时 %v）\n", len(decoded), decDur)

	// 4. 比对「结果」与原始内容。
	fmt.Println("\n[4] 结果比对（SHA-256 前 16 字节）：")
	decodedByName := make(map[string][]byte, len(decoded))
	for _, d := range decoded {
		decodedByName[d.Name] = d.Data
	}
	pass, fail := 0, 0
	for _, f := range files {
		want := sha256Hex(f.Data)
		got := ""
		resName := "(缺失)"
		if d, ok := decodedByName[f.Name]; ok {
			got = sha256Hex(d)
			resName = f.Name
		}
		ok := got != "" && string(want) == string(got)
		mark := "[一致]"
		if !ok {
			mark = "[不一致]"
			fail++
		} else {
			pass++
		}
		fmt.Printf("    %s %-32s 原始%s 还原%s  %s\n",
			mark, f.Name, want, got, resName)
	}

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
