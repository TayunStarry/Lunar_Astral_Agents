package main

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type FileCombiner struct {
	PNGTemplate string
	OutputFile  string
	Excluded    map[string]bool
}

func NewFileCombiner() *FileCombiner {
	return &FileCombiner{
		Excluded: make(map[string]bool),
	}
}

func (fc *FileCombiner) Combine() error {
	// 创建临时ZIP
	zipData, err := fc.createZipInMemory()
	if err != nil {
		return fmt.Errorf("create zip: %w", err)
	}

	// 读取PNG文件
	pngData, err := os.ReadFile(fc.PNGTemplate)
	if err != nil {
		return fmt.Errorf("read PNG: %w", err)
	}

	// 创建输出文件
	output, err := os.Create(fc.OutputFile)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer output.Close()

	// 写入PNG数据
	if _, err := output.Write(pngData); err != nil {
		return fmt.Errorf("write PNG: %w", err)
	}

	// 写入ZIP数据
	if _, err := output.Write(zipData); err != nil {
		return fmt.Errorf("write ZIP: %w", err)
	}

	return nil
}

func (fc *FileCombiner) createZipInMemory() ([]byte, error) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录
		if info.IsDir() {
			return nil
		}

		// 获取相对路径
		relPath, err := filepath.Rel(".", path)
		if err != nil {
			return err
		}

		// 检查是否排除
		if fc.shouldExclude(relPath) {
			return nil
		}

		// 添加到ZIP
		zipEntry, err := writer.Create(relPath)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(zipEntry, file)
		return err
	})

	if err != nil {
		return nil, err
	}

	writer.Close()
	return buf.Bytes(), nil
}

func (fc *FileCombiner) shouldExclude(path string) bool {
	// 基本排除
	if fc.Excluded[path] {
		return true
	}

	// 排除输出文件
	if path == fc.OutputFile {
		return true
	}

	// 排除PNG模板
	if path == fc.PNGTemplate {
		return true
	}

	// 排除ZIP文件
	if strings.HasSuffix(path, ".zip") {
		return true
	}

	return false
}

func main() {
	fc := NewFileCombiner()

	flag.StringVar(&fc.PNGTemplate, "png", "template.png", "PNG template file")
	flag.StringVar(&fc.OutputFile, "output", "combined.png", "Output file")
	flag.Parse()

	// 自动排除常见文件
	fc.Excluded[fc.PNGTemplate] = true
	fc.Excluded[fc.OutputFile] = true
	fc.Excluded[filepath.Base(os.Args[0])] = true

	// 创建组合文件
	if err := fc.Combine(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully created %s\n", fc.OutputFile)
	fmt.Printf("\nInstructions:\n")
	fmt.Printf("1. View as image:   open %s\n", fc.OutputFile)
	fmt.Printf("2. Extract files:   rename to .zip or use:\n")
	fmt.Printf("   unzip -q %s\n", fc.OutputFile)
}