package Creator

// dts.go 运行时 API 契约查阅：文档以 prompts/runtimeAPI.d.ts 的形式嵌入二进制，
// 运行期用 prompts.Read 读取，不再依赖可执行目录 / 工作目录下的散落文件。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/prompts"
)

// runtimeDocName 唯一的运行时 API 契约文档（嵌入在二进制内，文件名大小写须一致）。
const runtimeDocName = "runtimeAPI.d.ts"

// listDTSDocs 列出可查阅的运行时文档（只有一份嵌入契约）。
func listDTSDocs() ([]docInfo, error) {
	data, err := prompts.Read(runtimeDocName)
	if err != nil {
		return nil, err
	}
	return []docInfo{{Name: runtimeDocName, Size: int64(len(data))}}, nil
}

// readDTSDoc 读取内嵌运行时 API 契约；name 为空时返回文档清单。
// 只接受 runtimeAPI.d.ts 这一个精确文件名：不存在第二份文档，也不做任何旧称兼容。
func readDTSDoc(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		docs, err := listDTSDocs()
		if err != nil {
			return "", err
		}
		names := make([]string, 0, len(docs))
		for _, doc := range docs {
			names = append(names, fmt.Sprintf("%s（%d 字节）", doc.Name, doc.Size))
		}
		return "可用运行时文档：\n- " + strings.Join(names, "\n- "), nil
	}
	if name != runtimeDocName {
		return "", fmt.Errorf("只有一份运行时 API 契约：%s", runtimeDocName)
	}
	data, err := prompts.Read(runtimeDocName)
	if err != nil {
		return "", fmt.Errorf("读取运行时文档失败: %w", err)
	}
	return string(data), nil
}
