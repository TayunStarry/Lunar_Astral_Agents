// 演示 lunar-decoder 的编码与解码用法。
package main

import (
	"fmt"

	decoder "LunarSubsystem/LunarDecoder"
)

func main() {
	files := []decoder.FileData{
		{Name: "notes.txt", Data: []byte("星月智能 · 编码解码示例")},
		{Name: "binary.bin", Data: []byte{0x00, 0x01, 0x02, 0xff}},
	}

	// 使用 16 位整数密钥编码。
	const key int16 = 4321
	encoded, err := decoder.EncodeFiles(files, key)
	if err != nil {
		fmt.Println("编码失败:", err)
		return
	}
	for _, f := range encoded {
		fmt.Printf("已编码 [%s]: %d 字节\n", f.Name, len(f.Data))
	}

	// 使用相同密钥解码。
	decoded, err := decoder.DecodeFiles(encoded, key)
	if err != nil {
		fmt.Println("解码失败:", err)
		return
	}
	for _, f := range decoded {
		fmt.Printf("已还原 [%s]: %q\n", f.Name, string(f.Data))
	}

	// 查看密钥派生的各轮操作。
	rounds, err := decoder.KeyRounds(key)
	if err != nil {
		fmt.Println("密钥处理失败:", err)
		return
	}
	fmt.Printf("密钥 %d 拆分为 %d 轮:\n", key, len(rounds))
	for ri, r := range rounds {
		fmt.Printf(" 轮[%d] 密钥=%s\n", ri, r.Key)
		for i, g := range r.Groups {
			fmt.Printf("  组[%d] 类型=%d 次数=%d\n", i, g.Op, g.Count)
		}
	}
}
