package Painter

// promptdoc.go 按标题路径解析绘制者提示词文档。

import (
	"fmt"
	"regexp"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/prompts"
)

var headingRegex = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)

// readPrompt 读取内嵌提示词（失败返回空串）。
func readPrompt(name string) string {
	raw, err := prompts.Read(name)
	if err != nil {
		logger.Error(module, "读取提示词 %s 失败: %v", name, err)
		return ""
	}
	return string(raw)
}

// loadDocument 读取内嵌提示词并解析为标题路径文档。
func loadDocument(file string) *PromptDocument {
	content := readPrompt(file)
	doc := &PromptDocument{Sections: map[string]string{}, File: file}
	type heading struct {
		level int
		title string
	}
	stack := make([]heading, 0)
	currentPath := ""
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		match := headingRegex.FindStringSubmatch(line)
		if match != nil {
			level := len(match[1])
			title := strings.TrimSpace(match[2])
			filtered := make([]heading, 0, len(stack))
			for _, item := range stack {
				if item.level < level {
					filtered = append(filtered, item)
				}
			}
			stack = append(filtered, heading{level: level, title: title})
			parts := make([]string, 0)
			for _, item := range stack {
				if item.level >= 2 {
					parts = append(parts, item.title)
				}
			}
			currentPath = strings.Join(parts, "/")
			if currentPath != "" {
				if _, ok := doc.Sections[currentPath]; !ok {
					doc.Sections[currentPath] = ""
				}
			}
			continue
		}
		if currentPath == "" {
			continue
		}
		doc.Sections[currentPath] += line + "\n"
	}
	return doc
}

// text 读取某节正文。
func (doc *PromptDocument) text(path string) string {
	body, ok := doc.Sections[path]
	if !ok {
		panic(fmt.Sprintf("提示词文件 %s 缺少标题节「%s」", doc.File, path))
	}
	result := strings.TrimSpace(body)
	if result == "" {
		panic(fmt.Sprintf("提示词文件 %s 的标题节「%s」内容为空", doc.File, path))
	}
	return result
}

// list 读取某节列表项。
func (doc *PromptDocument) list(path string) []string {
	body := doc.text(path)
	items := make([]string, 0)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "*") {
			continue
		}
		line = strings.TrimSpace(line[1:])
		if line != "" {
			items = append(items, line)
		}
	}
	return items
}
