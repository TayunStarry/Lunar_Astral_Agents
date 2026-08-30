// 本程序演示 lunar-decoder 模块的编码与解码使用方式。
package main

import (
	"fmt"

	decoder "LunarSubsystem/LunarDecoder"
)

func main() {
	// 1. 构造原始文件数据。
	files := []decoder.FileData{
		{Name: "notes.txt", Data: []byte("星月智能 · 编码解码示例")},
		{Name: "binary.bin", Data: []byte{0x00, 0x01, 0x02, 0xff}},
	}

	// 2. 使用 16 位整数密钥编码。
	const key int16 = 4321
	encoded, err := decoder.EncodeFiles(files, key)
	if err != nil {
		fmt.Println("编码失败:", err)
		return
	}
	for _, f := range encoded {
		fmt.Printf("已编码 [%s]: %d 字节\n", f.Name, len(f.Data))
	}

	// 3. 使用相同密钥解码。
	decoded, err := decoder.DecodeFiles(encoded, key)
	if err != nil {
		fmt.Println("解码失败:", err)
		return
	}
	for i, f := range decoded {
		fmt.Printf("已还原 [%s]: %q\n", f.Name, string(f.Data))
		_ = i
	}

	// 4. 查看密钥派生的 8 组操作。
	groups, err := decoder.KeyGroups(key)
	if err != nil {
		fmt.Println("密钥处理失败:", err)
		return
	}
	fmt.Printf("密钥 %d 派生的 8 组操作:\n", key)
	for i, g := range groups {
		fmt.Printf(" 组[%d] 类型=%d 次数=%d\n", i, g.Op, g.Count)
	}
}
