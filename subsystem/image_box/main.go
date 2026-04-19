package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	ImageBox struct {
		Input  []string `json:"input"`
		Write  string   `json:"write"`
		Output string   `json:"output"`
	} `json:"image_box"`
}

type FileCombiner struct {
	PNGTemplate string
	OutputFile  string
	InputFiles  []string
	Excluded    map[string]bool
}

func NewFileCombiner() *FileCombiner {
	return &FileCombiner{
		Excluded: make(map[string]bool),
	}
}

func LoadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &config, nil
}

func (fc *FileCombiner) Combine() error {
	zipData, err := fc.createZipInMemory()
	if err != nil {
		return fmt.Errorf("create zip: %w", err)
	}

	pngData, err := os.ReadFile(fc.PNGTemplate)
	if err != nil {
		return fmt.Errorf("read PNG: %w", err)
	}

	output, err := os.Create(fc.OutputFile)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer output.Close()

	if _, err := output.Write(pngData); err != nil {
		return fmt.Errorf("write PNG: %w", err)
	}

	if _, err := output.Write(zipData); err != nil {
		return fmt.Errorf("write ZIP: %w", err)
	}

	return nil
}

func (fc *FileCombiner) createZipInMemory() ([]byte, error) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)

	for _, inputPath := range fc.InputFiles {
		err := filepath.Walk(inputPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			relPath, err := filepath.Rel(".", path)
			if err != nil {
				return err
			}

			if fc.shouldExclude(relPath) {
				return nil
			}

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
	}

	writer.Close()
	return buf.Bytes(), nil
}

func (fc *FileCombiner) shouldExclude(path string) bool {
	if fc.Excluded[path] {
		return true
	}

	if path == fc.OutputFile {
		return true
	}

	if path == fc.PNGTemplate {
		return true
	}

	if strings.HasSuffix(path, ".zip") {
		return true
	}

	return false
}

func main() {
	var inputFiles string
	var writeFile string
	var outputFile string

	flag.StringVar(&inputFiles, "input", "", "Input files (comma-separated)")
	flag.StringVar(&writeFile, "write", "", "PNG template file")
	flag.StringVar(&outputFile, "output", "", "Output file")
	flag.Parse()

	fc := NewFileCombiner()

	config, err := LoadConfig("./local_data/lunar_config.json")
	if err == nil {
		fc.PNGTemplate = config.ImageBox.Write
		fc.OutputFile = config.ImageBox.Output
		fc.InputFiles = config.ImageBox.Input
	}

	if inputFiles != "" {
		fc.InputFiles = strings.Split(inputFiles, ",")
	}
	if writeFile != "" {
		fc.PNGTemplate = writeFile
	}
	if outputFile != "" {
		fc.OutputFile = outputFile
	}

	fc.Excluded[fc.PNGTemplate] = true
	fc.Excluded[fc.OutputFile] = true
	fc.Excluded[filepath.Base(os.Args[0])] = true

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
