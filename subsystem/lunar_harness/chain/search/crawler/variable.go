package crawler

// variable.go 全局变量与常量集中声明（类型定义见 type.go）。

import (
	"regexp"
)

const (
	// webSearchPagesHardCap 页面条目总数的硬上限（含首页 1 条）
	webSearchPagesHardCap = 10
)

// engineFallbackOrder 搜索引擎降级顺序：bing → baidu → sogou
var engineFallbackOrder = []searchEngine{
	{
		name:      "bing",
		searchURL: "https://cn.bing.com/search?q=",
		container: "#b_results > li.b_algo",
		link:      "h2 a",
		snippet:   ".b_caption p, .b_caption, p",
	},
	{
		name:      "baidu",
		searchURL: "https://www.baidu.com/s?wd=",
		container: "#content_left > div",
		link:      "h3 a",
		snippet:   `.c-abstract, span[class*="content-right"], [class*="abstract"], p`,
	},
	{
		name:      "sogou",
		searchURL: "https://www.sogou.com/web?query=",
		container: "div.rb, div.vrwrap",
		link:      "h3 a",
		snippet:   ".str-text-info, .str_info, p",
	},
}

var (
	// pageCountRe 指令中的「前N个/页/条」数量要求
	pageCountRe = regexp.MustCompile(`前\s*([0-9]+|[０-９]+)\s*[个页条]`)
)
