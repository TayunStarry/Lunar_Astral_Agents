package ltpxmarket

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"LunarSubsystem/TuiSystem/channel"
)

// validateConfig 校验配置完备性。needToken 为 true 时（发布/写操作）要求令牌非空；
// 浏览/安装公开仓库属只读操作，可不带令牌（Gitee 读取 API 支持匿名访问）。
func validateConfig(cfg Config, needToken bool) error {
	if cfg.GiteeOwner == "" || cfg.GiteeRepo == "" {
		return fmt.Errorf("未配置 Gitee 仓库（ltpx_market.gitee_owner / gitee_repo，留空时回退 update_pack）")
	}
	if needToken && cfg.GiteeToken == "" {
		return fmt.Errorf("未提供 Gitee 令牌（发布需 Release 读写权限的私人令牌）")
	}
	return nil
}

// resolvePackageDir 解析 LTPX 包根目录（空时默认 local_data/package）。
// 相对路径一律以可执行文件目录为基准：程序可能自任意工作目录启动。
func resolvePackageDir(cfg Config) string {
	root := cfg.PackageDir
	if root == "" {
		root = filepath.Join("local_data", "package")
	}
	if filepath.IsAbs(root) {
		return root
	}
	if exe, err := os.Executable(); err == nil {
		if joined, err := filepath.Abs(filepath.Join(filepath.Dir(exe), root)); err == nil {
			return joined
		}
	}
	if abs, err := filepath.Abs(root); err == nil {
		return abs
	}
	return root
}

// ListLocalPackages 扫描包根目录下的全部 LTPX 包（*/metadata.json，按目录名排序）。
// 无 metadata.json 的子目录（资源/工具目录）跳过；包根目录不存在时返回空列表。
func ListLocalPackages(cfg Config) ([]LocalPackage, error) {
	dir := resolvePackageDir(cfg)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []LocalPackage{}, nil
		}
		return nil, fmt.Errorf("读取包目录失败: %w", err)
	}

	var pkgs []LocalPackage
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pkgDir := filepath.Join(dir, entry.Name())
		metaPath := filepath.Join(pkgDir, "metadata.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue // 非包目录
		}
		var meta PackageMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			logf("  [WARN] 解析 metadata.json 失败 %s: %v", metaPath, err)
			continue
		}
		pkgs = append(pkgs, LocalPackage{
			DirName: entry.Name(),
			Dir:     pkgDir,
			Meta:    meta,
			Size:    dirTotalSize(pkgDir),
		})
	}
	return pkgs, nil
}

// ListPackages 列举市场包：GET /releases → 过滤 [LTPX] 标记 → 解析 body →
// 标签筛选（大小写不敏感）→ 截取 limit（<=0 时默认 10）。tagFilter 为空时返回全部。
func ListPackages(cfg Config, tagFilter string, limit int) ([]MarketItem, error) {
	if err := validateConfig(cfg, false); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultTagLimit
	}
	// direction=desc 是必须的：Gitee 列表接口默认 asc（最旧在前）。此前依赖响应顺序
	// 又按 limit 截断，等于「只列最旧的 N 个市场包」，新发布的包根本进不了列表。
	query := url.Values{"per_page": {"100"}, "direction": {"desc"}}
	if cfg.GiteeToken != "" {
		query.Set("access_token", cfg.GiteeToken)
	}
	var releases []giteeRelease
	if err := apiRequest(fmt.Sprintf("/repos/%s/%s/releases", cfg.GiteeOwner, cfg.GiteeRepo), query, &releases); err != nil {
		return nil, err
	}
	// 参数之外再自己排一遍：取最新/截断都依赖「最新在前」，不能押在接口默认值上
	sort.SliceStable(releases, func(i, j int) bool {
		return channel.Newer(releases[i].CreatedAt, releases[i].TagName,
			releases[j].CreatedAt, releases[j].TagName)
	})
	logf("仓库共 %d 个发布，筛选市场包（标签: %s）", len(releases), orDash(tagFilter))

	filter := strings.ToLower(strings.TrimSpace(tagFilter))
	items := make([]MarketItem, 0, len(releases))
	foreign := 0
	for _, r := range releases {
		// 通道判别优先于标记匹配：更新包与市场共用同一仓库，
		// 必须显式排除更新包发布（双保险：正文无 [LTPX] 或附件前缀不符均跳过）
		if kind := releaseKind(r); kind != channel.Market {
			if kind == channel.Update {
				foreign++
			}
			continue
		}
		item := parseMarketBody(r)
		if item == nil {
			continue
		}
		if filter != "" && !itemHasTag(item, filter) {
			continue
		}
		items = append(items, *item)
		if len(items) >= limit {
			break
		}
	}
	if foreign > 0 {
		logf("  已跳过 %d 个更新包发布（非市场包）", foreign)
	}
	return items, nil
}

// itemHasTag 判断列表项是否命中标签筛选（大小写不敏感）
func itemHasTag(item *MarketItem, filter string) bool {
	for _, t := range item.Tags {
		if strings.EqualFold(strings.TrimSpace(t), filter) {
			return true
		}
	}
	return false
}

// parseMarketBody 解析市场 Release body 为列表项（非市场格式返回 nil）。
// 头部约定（行级识别，首个非头部行起为描述正文）：
//
//	[LTPX] <title>
//	ID: <id>
//	Tags: a, b
//	SHA-256: <hex>   ← 可空
func parseMarketBody(r giteeRelease) *MarketItem {
	if !strings.Contains(r.Body, bodyMarker) {
		return nil
	}
	// 行尾归一化：Gitee body 可能携带 CRLF，先统一为 LF 再逐行解析
	lines := strings.Split(strings.ReplaceAll(r.Body, "\r\n", "\n"), "\n")
	item := MarketItem{TagName: r.TagName}
	descStart := 0
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		isHeader := false
		switch {
		case strings.HasPrefix(line, bodyMarker):
			item.Title = strings.TrimSpace(strings.TrimPrefix(line, bodyMarker))
			isHeader = true
		case strings.HasPrefix(line, "ID:"):
			isHeader = true
		case strings.HasPrefix(line, "Tags:"):
			isHeader = true
			for _, t := range strings.Split(strings.TrimPrefix(line, "Tags:"), ",") {
				if t = strings.TrimSpace(t); t != "" {
					item.Tags = append(item.Tags, t)
				}
			}
		case strings.HasPrefix(line, "SHA-256:"):
			isHeader = true
			item.AssetSHA = strings.TrimSpace(strings.TrimPrefix(line, "SHA-256:"))
		}
		if isHeader {
			descStart = i + 1
			continue
		}
		if line == "" {
			continue // 头部与正文间的空行
		}
		descStart = i // 首个非头部非空行：描述正文开始
		break
	}
	item.Description = strings.TrimSpace(strings.Join(lines[descStart:], "\n"))

	// 附件：仅识别市场前缀附件（Gitee 自动附加的源码归档不带该前缀）
	for _, a := range r.Assets {
		if strings.HasPrefix(a.Name, assetPrefix) {
			item.AssetName = a.Name
			item.AssetURL = a.BrowserDownloadURL
			item.AssetSize = a.Size
			break
		}
	}
	return &item
}

// extractHashTags 从描述文本中识别 #标签：# 后连续的字母/数字/下划线/连字符/点/
// 非 ASCII 字符（中文等）构成标签，遇空白或标点结束。markdown 标题（# 后接空格）不误识别。
func extractHashTags(text string) []string {
	var tags []string
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '#' {
			continue
		}
		j := i + 1
		var buf []rune
		for ; j < len(runes) && isTagRune(runes[j]); j++ {
			buf = append(buf, runes[j])
		}
		if len(buf) > 0 {
			tags = append(tags, string(buf))
			i = j - 1
		}
	}
	return tags
}

// isTagRune 标签允许字符：字母（含中文）/数字/下划线/连字符/点
func isTagRune(r rune) bool {
	switch {
	case unicode.IsLetter(r), unicode.IsDigit(r):
		return true
	case r == '_' || r == '-' || r == '.':
		return true
	}
	return false
}

// mergeTags 合并标签：base 在前 extra 在后，大小写不敏感去重
func mergeTags(base, extra []string) []string {
	seen := make(map[string]bool, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, t := range append(append([]string{}, base...), extra...) {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	return out
}

// fileExists 检查文件/目录存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fileSize 取文件大小
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// dirTotalSize 递归统计目录总大小
func dirTotalSize(dir string) int64 {
	var total int64
	filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// orDash 空串显示为破折号
func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
