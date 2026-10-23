package ltpxmarket

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Install 从市场安装 LTPX 包：下载附件 → SHA 校验 → zip 解压（包根目录同卷暂存）→
// 校验 metadata.json → 旧目录备份到 package/.backup/<时间戳>/ → 改名落位。
// 返回落位后的包目录路径。落位目录名 = zip 顶层目录（发布端为原包目录名，随包保留；
// 前端经 resolvePackageDir 以 metadata.id 动态映射，id 与目录名可以不同）。
func Install(cfg Config, item MarketItem) (string, error) {
	if err := validateConfig(cfg, false); err != nil {
		return "", err
	}
	if item.AssetURL == "" {
		return "", fmt.Errorf("包 %s 没有可下载的安装附件", item.TagName)
	}
	reportProgress(-1)
	logf("开始安装 LTPX 包 %s（仓库 %s/%s）", item.TagName, cfg.GiteeOwner, cfg.GiteeRepo)
	pkgDir := resolvePackageDir(cfg)
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		return "", fmt.Errorf("创建包目录失败: %w", err)
	}

	// 暂存目录建在包根目录下（同卷）：落位用改名（零拷贝），
	// 避免系统 TEMP 跨卷导致移动失败（updatepack 已踩过的坑）
	staging, err := os.MkdirTemp(pkgDir, ".market_staging_")
	if err != nil {
		return "", fmt.Errorf("创建暂存目录失败: %w", err)
	}
	defer os.RemoveAll(staging)

	// ==== 下载（附件名取 Release 实际附件；兜底规范名 ltpx_<id>.ltpx） ====
	assetName := item.AssetName
	if assetName == "" {
		assetName = marketAssetName(item.TagName)
	}
	zipPath := filepath.Join(staging, assetName)
	logf("  下载安装包: %.1f MB", float64(item.AssetSize)/(1024*1024))
	if err := downloadFile(item.AssetURL, cfg.GiteeToken, zipPath, item.AssetSize); err != nil {
		return "", fmt.Errorf("下载安装包失败: %w", err)
	}
	reportProgress(60)

	// ==== SHA 校验（发布时声明过的） ====
	if item.AssetSHA != "" {
		sum, sErr := fileSHA256(zipPath)
		if sErr != nil {
			logf("  [WARN] 计算 SHA-256 失败，跳过校验: %v", sErr)
		} else if sum != item.AssetSHA {
			return "", fmt.Errorf("SHA-256 校验失败（期望 %s，实际 %s）", item.AssetSHA, sum)
		}
	}

	// ==== 解压 ====
	unpackDir := filepath.Join(staging, "unpack")
	if err := extractZip(zipPath, unpackDir); err != nil {
		return "", fmt.Errorf("解压失败: %w", err)
	}
	reportProgress(80)

	// ==== 定位包顶层目录：zip 内单顶层目录（发布端保证），散装条目则视为解压根 ====
	entries, err := os.ReadDir(unpackDir)
	if err != nil {
		return "", fmt.Errorf("检查解压内容失败: %w", err)
	}
	srcDir := unpackDir
	name := ""
	if len(entries) == 1 && entries[0].IsDir() {
		srcDir = filepath.Join(unpackDir, entries[0].Name())
		name = entries[0].Name()
	}

	// ==== 校验 metadata.json ====
	metaPath := filepath.Join(srcDir, "metadata.json")
	if !fileExists(metaPath) {
		return "", fmt.Errorf("安装包内缺少 metadata.json（不是有效的 LTPX 包）")
	}
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return "", fmt.Errorf("读取 metadata.json 失败: %w", err)
	}
	var meta PackageMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", fmt.Errorf("解析 metadata.json 失败: %w", err)
	}
	if meta.ID == "" {
		return "", fmt.Errorf("metadata.json 缺少 id 字段")
	}
	// 目录名回退：散装 zip 用包 id 规范化后落位
	if name == "" {
		name = sanitizeName(meta.ID)
	}

	// ==== 旧目录备份（改名失败=目录被占用，明确报错而非静默覆盖） ====
	dest := filepath.Join(pkgDir, name)
	if fileExists(dest) {
		backup := filepath.Join(pkgDir, ".backup", time.Now().Format("20060102-150405"), name)
		if err := os.MkdirAll(filepath.Dir(backup), 0755); err != nil {
			return "", fmt.Errorf("创建备份目录失败: %w", err)
		}
		if err := os.Rename(dest, backup); err != nil {
			return "", fmt.Errorf("旧版目录被占用，无法备份（%v）；请关闭相关页面后重试", err)
		}
		logf("  旧版已备份 → %s", backup)
	}

	// ==== 落位（同卷改名；极端情况下退回复制） ====
	if err := os.Rename(srcDir, dest); err != nil {
		if err := copyTree(srcDir, dest); err != nil {
			return "", fmt.Errorf("安装落位失败: %w", err)
		}
	}

	logf("✓ 已安装 %s（%s）→ %s", meta.Title, meta.ID, dest)
	reportProgress(100)
	return dest, nil
}

// extractZip 解压 zip 到目标目录（条目相对映射，防路径穿越）
func extractZip(zipPath, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	for _, f := range reader.File {
		cleaned := filepath.Clean(f.Name)
		if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
			logf("  [WARN] 跳过可疑路径: %s", f.Name)
			continue
		}
		dest := filepath.Join(destDir, cleaned)

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dest, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		if err := extractEntry(f, dest); err != nil {
			return fmt.Errorf("解压条目 %s 失败: %w", f.Name, err)
		}
	}
	return nil
}

// extractEntry 解压单个 zip 条目（流式拷贝）
func extractEntry(f *zip.File, dest string) error {
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, src)
	outErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return outErr
}

// copyTree 递归复制目录（落位改名的兜底通道）
func copyTree(srcDir, destDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rErr := filepath.Rel(srcDir, path)
		if rErr != nil {
			return rErr
		}
		dest := filepath.Join(destDir, rel)
		if info.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		return copyFile(path, dest)
	})
}

// copyFile 复制文件并显式关闭两端句柄（Windows 下句柄未释放时无法删除源文件）
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		_ = in.Close()
		return err
	}
	_, copyErr := io.Copy(out, in)
	outErr := out.Close()
	inErr := in.Close()
	if copyErr != nil {
		return copyErr
	}
	if outErr != nil {
		return outErr
	}
	return inErr
}

// sanitizeName 规范化包目录名：仅保留字母/数字/点/下划线/连字符，防止路径穿越
func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		}
	}
	if s := b.String(); s != "" {
		return s
	}
	return "unnamed-package"
}
