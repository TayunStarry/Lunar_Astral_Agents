package Creator

// dts.go 运行时 API 契约查阅：契约按能力拆分为多个 prompts/*API.d.ts 并嵌入二进制，
// 运行期用 prompts.ListDTS / prompts.Read 列出与读取，便于按需只加载所需模块。

import (
	"fmt"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/prompts"
)

// listDTSDocs 列出可查阅的运行时文档（全部内嵌 *API.d.ts，按名称升序）。
func listDTSDocs() ([]docInfo, error) {
	docs, err := prompts.ListDTS()
	if err != nil {
		return nil, err
	}
	out := make([]docInfo, 0, len(docs))
	for _, doc := range docs {
		out = append(out, docInfo{Name: doc.Name, Size: doc.Size})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// readDTSDoc 读取内嵌运行时 API 契约；name 为空时返回文档清单。
// 只接受清单中的精确文件名，不做前缀 / 大小写 / 旧称兼容。
func readDTSDoc(name string) (string, error) {
	name = strings.TrimSpace(name)
	docs, err := listDTSDocs()
	if err != nil {
		return "", err
	}
	if name == "" {
		names := make([]string, 0, len(docs))
		for _, doc := range docs {
			names = append(names, fmt.Sprintf("%s（%d 字节）", doc.Name, doc.Size))
		}
		return "可用运行时文档：\n- " + strings.Join(names, "\n- "), nil
	}
	for _, doc := range docs {
		if doc.Name != name {
			continue
		}
		data, rerr := prompts.Read(name)
		if rerr != nil {
			return "", fmt.Errorf("读取运行时文档失败: %w", rerr)
		}
		return string(data), nil
	}
	available := make([]string, 0, len(docs))
	for _, doc := range docs {
		available = append(available, doc.Name)
	}
	return "", fmt.Errorf("未知的运行时 API 文档 %q；可用：%s", name, strings.Join(available, "、"))
}
