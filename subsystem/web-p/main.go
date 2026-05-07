package main

import (
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/chai2010/webp"
)

func main() {
	fmt.Println("=== PNG to WebP Converter ===")
	fmt.Println("Scanning current directory for PNG files...")

	currentDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error: Failed to get current directory: %v\n", err)
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
		fmt.Printf("Error: Failed to scan directory: %v\n", err)
		os.Exit(1)
	}

	if len(pngFiles) == 0 {
		fmt.Println("No PNG files found in the current directory.")
		return
	}

	fmt.Printf("Found %d PNG file(s):\n", len(pngFiles))
	for _, f := range pngFiles {
		fmt.Printf("  - %s\n", filepath.Base(f))
	}

	fmt.Println("\nStarting conversion...")
	successCount := 0
	failCount := 0

	for _, pngPath := range pngFiles {
		webpPath := strings.TrimSuffix(pngPath, filepath.Ext(pngPath)) + ".webp"
		
		fmt.Printf("\nConverting: %s -> %s\n", filepath.Base(pngPath), filepath.Base(webpPath))

		err := convertPNGToWebP(pngPath, webpPath, 90)
		if err != nil {
			fmt.Printf("  FAILED: %v\n", err)
			failCount++
		} else {
			fmt.Println("  SUCCESS")
			successCount++
		}
	}

	fmt.Println("\n=== Conversion Complete ===")
	fmt.Printf("Success: %d | Failed: %d\n", successCount, failCount)

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
