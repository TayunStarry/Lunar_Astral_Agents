package main

import (
	"fmt"
	"image"
	_ "image/png"
	"logger"
	"os"
	"path/filepath"
	"strings"

	"github.com/chai2010/webp"
)

func main() {
	logger.SetDevMode(true)
	logger.Info("WebPConv", "=== PNG to WebP Converter ===")
	logger.Info("WebPConv", "Scanning current directory for PNG files...")

	currentDir, err := os.Getwd()
	if err != nil {
		logger.Error("WebPConv", "Failed to get current directory: %v", err)
		os.Exit(1)
	}

	var pngFiles []string
	err = filepath.Walk(currentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.EqualFold(filepath.Ext(path), ".png") {
			pngFiles = append(pngFiles, path)
		}
		return nil
	})

	if err != nil {
		logger.Error("WebPConv", "Failed to scan directory: %v", err)
		os.Exit(1)
	}

	if len(pngFiles) == 0 {
		logger.Info("WebPConv", "No PNG files found in the current directory.")
		return
	}

	logger.Info("WebPConv", "Found %d PNG file(s)", len(pngFiles))
	for _, f := range pngFiles {
		logger.Info("WebPConv", "  - %s", filepath.Base(f))
	}

	logger.Info("WebPConv", "Starting conversion...")
	successCount := 0
	failCount := 0

	for _, pngPath := range pngFiles {
		webpPath := strings.TrimSuffix(pngPath, filepath.Ext(pngPath)) + ".webp"

		logger.Info("WebPConv", "Converting: %s -> %s", filepath.Base(pngPath), filepath.Base(webpPath))

		err := convertPNGToWebP(pngPath, webpPath, 90)
		if err != nil {
			logger.Error("WebPConv", "FAILED: %v", err)
			failCount++
		} else {
			logger.Info("WebPConv", "SUCCESS")
			successCount++
		}
	}

	logger.Info("WebPConv", "=== Conversion Complete ===")
	logger.Info("WebPConv", "Success: %d | Failed: %d", successCount, failCount)

	if failCount > 0 {
		os.Exit(1)
	}
}

func convertPNGToWebP(inputPath, outputPath string, quality int) error {
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %v", err)
	}
	defer inputFile.Close()

	img, _, err := image.Decode(inputFile)
	if err != nil {
		return fmt.Errorf("failed to decode PNG image: %v", err)
	}

	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outputFile.Close()

	err = webp.Encode(outputFile, img, &webp.Options{Quality: float32(quality)})
	if err != nil {
		return fmt.Errorf("failed to encode WebP image: %v", err)
	}

	return nil
}
