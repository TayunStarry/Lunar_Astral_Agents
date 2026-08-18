package module

import (
	"LunarSubsystem/LoggerGeneral"
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
)

// CreateZip 创建 ZIP 压缩文件并返回字节数据
func CreateZip(files []*multipart.FileHeader, zipName string) ([]byte, error) {
	// 检查文件列表是否为空
	if len(files) == 0 {
		return nil, fmt.Errorf("未选择文件")
	}
	// 确保 ZIP 文件名以 ".zip" 结尾
	if zipName == "" {
		zipName = "archive.zip"
	}
	if !strings.HasSuffix(strings.ToLower(zipName), ".zip") {
		zipName += ".zip"
	}
	// 创建一个内存缓冲区，用于存储 ZIP 文件内容
	var zipBuffer bytes.Buffer
	// 创建一个新的 ZIP 写入器，将内容写入缓冲区
	zipWriter := zip.NewWriter(&zipBuffer)
	// 遍历所有要压缩的文件
	for _, fileHeader := range files {
		// 打开表单中的文件
		file, openErr := fileHeader.Open()
		if openErr != nil {
			return nil, fmt.Errorf("打开文件失败: %w", openErr)
		}
		// 创建 ZIP 文件头，设置文件名和压缩方法为 Deflate
		zipHeader := &zip.FileHeader{
			Name:   fileHeader.Filename,
			Method: zip.Deflate,
		}
		// 在 ZIP 文件中创建一个新的条目
		zipFileWriter, createErr := zipWriter.CreateHeader(zipHeader)
		if createErr != nil {
			// 若创建条目失败，关闭已打开的文件并返回错误
			file.Close()
			return nil, fmt.Errorf("创建ZIP条目失败: %w", createErr)
		}
		// 将文件内容复制到 ZIP 条目中
		_, err := io.Copy(zipFileWriter, file)
		// 关闭已打开的文件，释放资源
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("写入ZIP内容失败: %w", err)
		}
	}
	// 关闭 ZIP 写入器，完成 ZIP 文件的创建
	err := zipWriter.Close()
	if err != nil {
		return nil, fmt.Errorf("关闭ZIP写入器失败: %w", err)
	}
	LoggerGeneral.SubInfo("FileManager", "Archive", "成功创建ZIP文件: %s, 包含 %d 个文件", zipName, len(files))
	// 从缓冲区获取 ZIP 文件的字节数据
	return zipBuffer.Bytes(), nil
}

// ExtractZip 解压 ZIP 文件并返回文件列表
func ExtractZip(file multipart.File) ([]map[string]any, string, error) {
	// 在内存中读取 ZIP 文件的全部内容
	zipData, err := io.ReadAll(file)
	if err != nil {
		return nil, "", fmt.Errorf("读取ZIP文件失败: %w", err)
	}
	// 从字节数据创建一个 ZIP 读取器，用于后续解压操作
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, "", fmt.Errorf("打开ZIP文件失败: %w", err)
	}
	// 用于存储解压后的文件信息
	var extractedFiles []map[string]any
	// 遍历 ZIP 读取器中的所有文件
	for _, zipFile := range zipReader.File {
		// 跳过目录，只处理文件
		if zipFile.FileInfo().IsDir() {
			continue
		}
		// 打开 ZIP 中的文件，获取一个可读的文件句柄
		rc, err := zipFile.Open()
		if err != nil {
			return nil, "", fmt.Errorf("打开ZIP内文件失败: %w", err)
		}
		// 读取 ZIP 中文件的全部内容
		fileContent, err := io.ReadAll(rc)
		// 关闭文件句柄，释放资源
		rc.Close()
		if err != nil {
			return nil, "", fmt.Errorf("读取ZIP内文件内容失败: %w", err)
		}
		// 获取文件的基本信息
		fileInfo := zipFile.FileInfo()
		// 将解压后的文件信息添加到列表中
		extractedFiles = append(extractedFiles, map[string]any{
			"name":          zipFile.Name,                                // 文件名
			"size":          fileInfo.Size(),                             // 文件大小
			"content":       fileContent,                                 // 文件内容
			"last_modified": fileInfo.ModTime(),                          // 最后修改时间
			"extension":     strings.ToLower(filepath.Ext(zipFile.Name)), // 文件扩展名
		})
	}
	return extractedFiles, zipReader.File[0].Name, nil
}

// PackageDirZip 将指定目录打包为 ZIP 字节数据
// dirPath: 要打包的目录路径
// packageName: 包名，作为 ZIP 内文件的根目录前缀
func PackageDirZip(dirPath string, packageName string) ([]byte, error) {
	// 检查目录是否存在
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("目录不存在: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("路径不是目录: %s", dirPath)
	}

	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// 遍历目录下的所有文件
	err = filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// 跳过根目录本身
		if path == dirPath {
			return nil
		}
		// 跳过子目录（目录条目会在文件写入时自动创建）
		if d.IsDir() {
			return nil
		}

		// 获取相对于包目录的路径
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return fmt.Errorf("获取相对路径失败: %w", err)
		}

		// ZIP 内部路径：包名/相对路径
		zipPath := filepath.ToSlash(filepath.Join(packageName, relPath))

		// 创建 ZIP 条目
		zipEntry, err := zipWriter.Create(zipPath)
		if err != nil {
			return fmt.Errorf("创建 ZIP 条目失败 %s: %w", zipPath, err)
		}

		// 读取并写入文件内容
		fileContent, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("读取文件失败 %s: %w", path, err)
		}

		if _, err := zipEntry.Write(fileContent); err != nil {
			return fmt.Errorf("写入 ZIP 条目失败 %s: %w", zipPath, err)
		}

		return nil
	})

	if err != nil {
		zipWriter.Close()
		return nil, fmt.Errorf("打包目录失败: %w", err)
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("关闭 ZIP 写入器失败: %w", err)
	}

	LoggerGeneral.SubInfo("FileManager", "Archive", "成功打包目录: %s -> %s.ltpx (%d 字节)", dirPath, packageName, buf.Len())
	return buf.Bytes(), nil
}

// CreateZipFromPaths 将服务器本地的文件/目录路径列表打包为 ZIP 字节数据
// localDir: LocalDir 绝对路径；paths: 相对 localDir 的文件/目录路径列表
// ZIP 内部路径保留相对 localDir 的层级结构（目录条目一并打包，保留空目录）
func CreateZipFromPaths(localDir string, paths []string, zipName string) ([]byte, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("未选择文件")
	}
	if zipName == "" {
		zipName = "archive.zip"
	}
	if !strings.HasSuffix(strings.ToLower(zipName), ".zip") {
		zipName += ".zip"
	}

	baseDir := filepath.Clean(localDir)
	basePrefix := baseDir + string(os.PathSeparator)
	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)

	// 打包单个相对路径（文件或目录），ZIP 内路径保留相对 localDir 的层级
	packOne := func(relPath string) error {
		// 安全校验：拼接后的绝对路径必须位于 LocalDir 内（防目录遍历）
		fullPath := filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(relPath)))
		if fullPath != baseDir && !strings.HasPrefix(fullPath, basePrefix) {
			return fmt.Errorf("路径越界: %s", relPath)
		}
		info, err := os.Stat(fullPath)
		if err != nil {
			return fmt.Errorf("路径不存在: %s: %w", relPath, err)
		}
		// ZIP 内路径（去除 baseDir 前缀，统一 "/" 分隔符）
		zipRel := strings.TrimPrefix(fullPath, baseDir)
		zipRel = strings.TrimPrefix(filepath.ToSlash(zipRel), "/")
		if zipRel == "" {
			return fmt.Errorf("无效路径: %s", relPath)
		}

		if !info.IsDir() {
			// 单文件：直接写入 ZIP
			zipEntry, createErr := zipWriter.Create(zipRel)
			if createErr != nil {
				return fmt.Errorf("创建ZIP条目失败 %s: %w", zipRel, createErr)
			}
			src, openErr := os.Open(fullPath)
			if openErr != nil {
				return fmt.Errorf("打开文件失败 %s: %w", fullPath, openErr)
			}
			_, copyErr := io.Copy(zipEntry, src)
			src.Close()
			if copyErr != nil {
				return fmt.Errorf("写入ZIP内容失败 %s: %w", zipRel, copyErr)
			}
			return nil
		}

		// 目录：递归打包，ZIP 内保留目录层级（含文件夹名）
		return filepath.WalkDir(fullPath, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, relErr := filepath.Rel(fullPath, path)
			if relErr != nil {
				return fmt.Errorf("获取相对路径失败: %w", relErr)
			}
			zipPath := zipRel
			if rel != "." {
				zipPath = filepath.ToSlash(filepath.Join(zipRel, rel))
			}
			if d.IsDir() {
				// 目录条目以 "/" 结尾，保留空目录
				_, createErr := zipWriter.Create(zipPath + "/")
				if createErr != nil {
					return fmt.Errorf("创建ZIP目录条目失败 %s: %w", zipPath, createErr)
				}
				return nil
			}
			zipEntry, createErr := zipWriter.Create(zipPath)
			if createErr != nil {
				return fmt.Errorf("创建ZIP条目失败 %s: %w", zipPath, createErr)
			}
			src, openErr := os.Open(path)
			if openErr != nil {
				return fmt.Errorf("打开文件失败 %s: %w", path, openErr)
			}
			_, copyErr := io.Copy(zipEntry, src)
			src.Close()
			if copyErr != nil {
				return fmt.Errorf("写入ZIP内容失败 %s: %w", zipPath, copyErr)
			}
			return nil
		})
	}

	for _, p := range paths {
		if err := packOne(p); err != nil {
			zipWriter.Close()
			return nil, err
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("关闭ZIP写入器失败: %w", err)
	}
	LoggerGeneral.SubInfo("FileManager", "Archive", "成功创建ZIP文件: %s, 包含 %d 个路径", zipName, len(paths))
	return zipBuffer.Bytes(), nil
}

// ReadZipFileList 读取 ZIP 文件内条目信息（不读取文件内容）
// zipPath: ZIP 文件绝对路径
func ReadZipFileList(zipPath string) ([]ZipEntryInfo, error) {
	zipReader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("打开ZIP文件失败: %w", err)
	}
	defer zipReader.Close()

	entries := make([]ZipEntryInfo, 0, len(zipReader.File))
	for _, zipFile := range zipReader.File {
		info := zipFile.FileInfo()
		entries = append(entries, ZipEntryInfo{
			Name:       zipFile.Name,
			Size:       info.Size(),
			Compressed: int64(zipFile.CompressedSize64),
			IsDir:      info.IsDir(),
		})
	}
	return entries, nil
}

// ExtractZipToDir 将 ZIP 文件解压到目标目录
// zipPath: ZIP 文件绝对路径；targetDir: 目标目录绝对路径（需已校验在 LocalDir 内）
// 返回解压出的文件数
func ExtractZipToDir(zipPath string, targetDir string) (int, error) {
	zipReader, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, fmt.Errorf("打开ZIP文件失败: %w", err)
	}
	defer zipReader.Close()

	// 创建目标根目录
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return 0, fmt.Errorf("创建解压目录失败: %w", err)
	}

	baseDir := filepath.Clean(targetDir) + string(os.PathSeparator)
	fileCount := 0
	for _, zipFile := range zipReader.File {
		// 防止 ZIP 路径穿越（zip-slip）
		destPath := filepath.Clean(filepath.Join(targetDir, filepath.FromSlash(zipFile.Name)))
		if destPath != filepath.Clean(targetDir) && !strings.HasPrefix(destPath, baseDir) {
			LoggerGeneral.SubWarn("FileManager", "Archive", "跳过越界条目: %s", zipFile.Name)
			continue
		}

		if zipFile.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fileCount, fmt.Errorf("创建目录失败 %s: %w", destPath, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fileCount, fmt.Errorf("创建父目录失败 %s: %w", destPath, err)
		}

		rc, err := zipFile.Open()
		if err != nil {
			return fileCount, fmt.Errorf("打开ZIP内文件失败 %s: %w", zipFile.Name, err)
		}
		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return fileCount, fmt.Errorf("创建输出文件失败 %s: %w", destPath, err)
		}
		_, copyErr := io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if copyErr != nil {
			return fileCount, fmt.Errorf("写入文件失败 %s: %w", destPath, copyErr)
		}
		fileCount++
	}
	return fileCount, nil
}
