package Creator

// tools_files.go 文件与代码索引工具：全部限定在 /document 根目录。

import (
	"fmt"
	"strings"
)

func fileList(args map[string]any) toolOutcome {
	entries, err := listDocuments(argString(args, "path"))
	if err != nil {
		return failure(toolFileList, err)
	}
	return toolOutcome{
		JSON:    jsonText(entries),
		Summary: fmt.Sprintf("列出 %d 个条目", len(entries)),
	}
}

func fileRead(args map[string]any) toolOutcome {
	content, err := readDocument(argString(args, "path"))
	if err != nil {
		return failure(toolFileRead, err)
	}
	return toolOutcome{
		JSON:    jsonText(map[string]any{"path": argString(args, "path"), "content": content}),
		Summary: "读取文件 " + argString(args, "path"),
	}
}

func fileCreate(args map[string]any) toolOutcome {
	if _, err := createDocument(argString(args, "path"), argString(args, "content"), true); err != nil {
		return failure(toolFileCreate, err)
	}
	return toolOutcome{
		JSON:    jsonText(map[string]any{"path": argString(args, "path"), "created": true}),
		Summary: "创建文件 " + argString(args, "path"),
	}
}

func fileRewrite(args map[string]any) toolOutcome {
	if _, err := createDocument(argString(args, "path"), argString(args, "content"), false); err != nil {
		return failure(toolFileRewrite, err)
	}
	return toolOutcome{
		JSON:    jsonText(map[string]any{"path": argString(args, "path"), "rewritten": true}),
		Summary: "重写文件 " + argString(args, "path"),
	}
}

func fileRemove(args map[string]any) toolOutcome {
	if err := removeDocumentContent(argString(args, "path")); err != nil {
		return failure(toolFileRemove, err)
	}
	return toolOutcome{
		JSON:    jsonText(map[string]any{"path": argString(args, "path"), "cleared": true}),
		Summary: "清空文件 " + argString(args, "path"),
	}
}

func fileDelete(args map[string]any) toolOutcome {
	if err := deleteDocument(argString(args, "path")); err != nil {
		return failure(toolFileDelete, err)
	}
	return toolOutcome{
		JSON:    jsonText(map[string]any{"path": argString(args, "path"), "deleted": true}),
		Summary: "删除文件 " + argString(args, "path"),
	}
}

func fileMove(args map[string]any) toolOutcome {
	if err := moveDocument(argString(args, "path"), argString(args, "target")); err != nil {
		return failure(toolFileMove, err)
	}
	return toolOutcome{
		JSON:    jsonText(map[string]any{"from": argString(args, "path"), "to": argString(args, "target")}),
		Summary: "移动文件到 " + argString(args, "target"),
	}
}

func fileReplace(args map[string]any) toolOutcome {
	count, err := replaceDocument(argString(args, "path"), argString(args, "search"), argString(args, "replace"), argBool(args, "all"))
	if err != nil {
		return failure(toolFileReplace, err)
	}
	return toolOutcome{
		JSON:    jsonText(map[string]any{"path": argString(args, "path"), "replaced": count}),
		Summary: fmt.Sprintf("替换 %d 处内容", count),
	}
}

func fileDeleteText(args map[string]any) toolOutcome {
	count, err := deleteDocumentText(argString(args, "path"), argString(args, "text"), argBool(args, "all"))
	if err != nil {
		return failure(toolFileDeleteText, err)
	}
	return toolOutcome{
		JSON:    jsonText(map[string]any{"path": argString(args, "path"), "deleted": count}),
		Summary: fmt.Sprintf("删除 %d 处内容", count),
	}
}

func codeIndexTool(args map[string]any) toolOutcome {
	content, err := readDocument(argString(args, "path"))
	if err != nil {
		return failure(toolCodeIndex, err)
	}
	index := buildCodeIndex(content)
	return toolOutcome{
		JSON:    jsonText(index),
		Summary: fmt.Sprintf("代码索引：%d 变量 / %d 函数 / %d 类", len(index.Variables), len(index.Functions), len(index.Classes)),
	}
}

func readDTSTool(args map[string]any) toolOutcome {
	name := strings.TrimSpace(argString(args, "name"))
	content, err := readDTSDoc(name)
	if err != nil {
		return failure(toolReadDTS, err)
	}
	return toolOutcome{
		JSON:      jsonText(map[string]any{"name": name, "content": content}),
		Summary:   "查阅 DTS 文档 " + previewRunes(name, 40),
		Reference: "DTS 文档《" + previewRunes(name, 60) + "》：" + content,
	}
}
