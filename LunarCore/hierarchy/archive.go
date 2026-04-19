package hierarchy

import (
	"LunarCore/config"
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
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
	// 记录日志，包含创建的 ZIP 文件名和包含的文件数量
	if *config.Developer {
		log.Printf("%s", strings.Repeat("-=", 28))
		log.Printf("Archive请求 -> 成功创建ZIP文件: %s, 包含 %d 个文件", zipName, len(files))
		log.Printf("%s", strings.Repeat("-=", 28))
	}
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
