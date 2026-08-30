# 子系统——文件编解码器（lunar_decoder）

轻量级文件编码/解码模块，对文件二进制数据执行基于 16 位整数密钥的确定性可逆变换，用于数据的简单混淆与还原。

## 功能概述

| 特性 | 说明 |
|------|------|
| 模块名 | `lunar_decoder`（`LunarSubsystem/LunarDecoder`） |
| 输入 | `[]FileData` 文件数据数组 + `int16` 密钥 |
| 编码 | 二进制 → 密钥绑定游程编码 → Base64(RFC 4648) → 按密钥派生的 8 组操作字符变换 |
| 解码 | 严格逆序还原（含游程解码）并解码回原始二进制数据 |
| 可逆性 | 编码/解码往返完全一致（字节级双射） |
| 产物文本性 | 编码产物恒为 Base64 字符集（含填充 `=`）内的可打印文本，可安全存入 JSON / 文本传输场景 |
| 依赖 | 仅使用 Go 标准库 |

## 使用方式

```go
import decoder "LunarSubsystem/LunarDecoder"

files := []decoder.FileData{
    {Name: "a.txt", Data: []byte("hello")},
}
const key int16 = 4321

encoded, _ := decoder.EncodeFiles(files, key)
decoded, _ := decoder.DecodeFiles(encoded, key) // 还原原始 bytes
```

完整示例见 [cmd/example/main.go](cmd/example/main.go)。

## API 文档

### `type FileData`
| 字段 | 类型 | 说明 |
|------|------|------|
| `Name` | `string` | 文件名，编解码原样保留 |
| `Data` | `[]byte` | 原始二进制数据或编码后的 Base64 文本字节 |

### `type Group`
| 字段 | 类型 | 说明 |
|------|------|------|
| `Op` | `int` | 操作类型（0-9），见下表 |
| `Count` | `int` | 操作次数（0-9） |

**操作类型对照**：

| 类型 | 目标范围 | 操作 |
|------|---------|------|
| 0 | 数字 0-9 | 向后循环位移 |
| 1 | 数字 0-9 | 向前循环位移 |
| 2 | 字母 a-zA-Z | 向后循环位移 |
| 3 | 字母 a-zA-Z | 向前循环位移 |
| 4 | 符号 +、/、= | 向后循环位移 |
| 5 | 符号 +、/、= | 向前循环位移 |
| 6 | 全部字符 | 反向排序表转换 + 向后循环位移 |
| 7 | 全部字符 | 反向排序表转换 + 向前循环位移 |
| 8 | 全部字符 | 与下一位字符的字符集索引异或 + 向后循环位移 |
| 9 | 全部字符 | 与下一位字符的字符集索引异或 + 向前循环位移 |

### `func EncodeFiles(files []FileData, key int16) ([]FileData, error)`
对文件数组编码。转换顺序：Base64 → 按组 0..7 正向操作。若 `files` 或任一 `Data` 为 nil 返回 `ErrNilData`。

### `func DecodeFiles(files []FileData, key int16) ([]FileData, error)`
对编码数组解码。转换顺序：按组 7..0 逆操作 → Base64 解码。还原内容非法时返回 `ErrInvalidBase64Category`。

### `func KeyGroups(key int16) ([8]Group, error)`
将密钥标准化（16 位、逐位不进位加法合并）并拆分为 8 组操作。

### `func ApplyTransform(s []byte, groups [8]Group, decode bool) []byte`
按 8 组操作对字节串变换；`decode=false` 编码，`true` 解码。

## 运行与测试

```sh
# 运行示例
go run ./cmd/example

# 单元测试（含覆盖率）
go test ./... -cover
```

## 注意事项

- 密钥取 16 位整数的无符号幅度参与分组，避免负号干扰数字分组。
- 编码前对二进制数据执行密钥绑定的游程编码（RLE）：控制字节由密钥派生、转义规则内建，格式非通用（普通 RLE 工具无法解析），作为编码链路上的额外混淆层；游程最小长度 `rleMinRun=4`，低于该长度的重复按字面量输出，对已压缩媒体文件几乎不改变体积。
- 操作 8/9 的相邻异或作用于 64 字符 Base64 字符集的索引上（两个 6 位索引异或结果仍是 6 位），保证异或产物不越出字符集；字符集外的填充 `=` 作为边界原样保留。末位不参与异或，以保证严格可逆。
- 依赖

- Go 标准库（encoding/base64、strconv、strings 等）。

## 相关文档

- [项目主文档](../../README.md)
- [前端开发指南](../../.trae/rules/frontend-development-guide.md)（类型/变量文件职责分离约定）