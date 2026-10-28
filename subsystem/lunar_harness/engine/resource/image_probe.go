package resource

// image_probe.go 从文件头解析图片宽高（PNG / JPEG / GIF / WebP / BMP），
// 不引入解码依赖、不整图入内存。

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// imageDimensions 解析图片文件的宽高（支持 PNG / JPEG / GIF / WebP / BMP）
func imageDimensions(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	head := make([]byte, 32)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return 0, 0, err
	}
	head = head[:n]
	if len(head) < 16 {
		return 0, 0, fmt.Errorf("文件过短，无法解析图片头")
	}
	switch {
	case isPNG(head):
		// PNG 签名(8) + IHDR 长度(4) + "IHDR"(4) 之后，宽度/高度位于偏移 16..24（大端）
		if len(head) >= 24 {
			return int(binary.BigEndian.Uint32(head[16:20])), int(binary.BigEndian.Uint32(head[20:24])), nil
		}
	case isGIF(head):
		return int(binary.LittleEndian.Uint16(head[6:8])), int(binary.LittleEndian.Uint16(head[8:10])), nil
	case isBMP(head):
		if len(head) >= 26 {
			return int(int32(binary.LittleEndian.Uint32(head[18:22]))), int(int32(binary.LittleEndian.Uint32(head[22:26]))), nil
		}
	case isWebP(head):
		return webPDimensions(path)
	case isJPEG(head):
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return 0, 0, err
		}
		return jpegDimensions(f)
	}
	return 0, 0, fmt.Errorf("不支持的图片格式")
}

func isPNG(head []byte) bool {
	return len(head) >= 8 && head[0] == 0x89 && head[1] == 'P' && head[2] == 'N' && head[3] == 'G'
}

func isGIF(head []byte) bool {
	return len(head) >= 6 && head[0] == 'G' && head[1] == 'I' && head[2] == 'F'
}

func isBMP(head []byte) bool {
	return len(head) >= 2 && head[0] == 'B' && head[1] == 'M'
}

func isWebP(head []byte) bool {
	return len(head) >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "WEBP"
}

func isJPEG(head []byte) bool {
	return len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8
}

// webPDimensions 解析 WebP 容器尺寸（覆盖 VP8 / VP8L / VP8X 三种块）
func webPDimensions(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	// 跳过 RIFF(4) + 长度(4) + WEBP(4) + 块标识(4) + 块长度(4) = 20 字节
	head := make([]byte, 20+10)
	if _, err := io.ReadFull(f, head); err != nil {
		return 0, 0, fmt.Errorf("WebP 头读取不足: %v", err)
	}
	format := string(head[12:16])
	chunk := head[20:]
	switch format {
	case "VP8 ":
		if len(chunk) >= 10 {
			w := int(binary.LittleEndian.Uint16(chunk[6:8]) & 0x3FFF)
			h := int(binary.LittleEndian.Uint16(chunk[8:10]) & 0x3FFF)
			return w, h, nil
		}
	case "VP8L":
		if len(chunk) >= 5 {
			bits := binary.LittleEndian.Uint32(chunk[1:5])
			w := int(bits&0x3FFF) + 1
			h := int((bits>>14)&0x3FFF) + 1
			return w, h, nil
		}
	case "VP8X":
		if len(chunk) >= 10 {
			w := int(uint32(chunk[4])|uint32(chunk[5])<<8|uint32(chunk[6])<<16) + 1
			h := int(uint32(chunk[7])|uint32(chunk[8])<<8|uint32(chunk[9])<<16) + 1
			return w, h, nil
		}
	}
	return 0, 0, fmt.Errorf("无法解析 WebP 尺寸")
}

// jpegDimensions 顺序扫描 JPEG 段，定位 SOFn 帧头读取宽高
func jpegDimensions(r io.Reader) (int, int, error) {
	reader := bufio.NewReader(r)
	// 跳过 SOI
	if _, err := reader.Discard(2); err != nil {
		return 0, 0, err
	}
	for {
		marker, err := readJPEGMarker(reader)
		if err != nil {
			return 0, 0, err
		}
		// 无载荷的标记（RSTn / TEM）继续扫描
		if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			continue
		}
		lengthBytes := make([]byte, 2)
		if _, err := io.ReadFull(reader, lengthBytes); err != nil {
			return 0, 0, err
		}
		length := int(binary.BigEndian.Uint16(lengthBytes))
		if length < 2 {
			return 0, 0, fmt.Errorf("JPEG 段长度非法")
		}
		// SOF0..SOF15（0xC0..0xCF，排除 DHT 0xC4 / JPG 0xC8 / DAC 0xCC）
		if marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
			payload := make([]byte, length-2)
			if _, err := io.ReadFull(reader, payload); err != nil {
				return 0, 0, err
			}
			if len(payload) < 5 {
				return 0, 0, fmt.Errorf("JPEG SOF 段过短")
			}
			h := int(binary.BigEndian.Uint16(payload[1:3]))
			w := int(binary.BigEndian.Uint16(payload[3:5]))
			return w, h, nil
		}
		if _, err := reader.Discard(length - 2); err != nil {
			return 0, 0, err
		}
	}
}

// readJPEGMarker 读取下一个 JPEG 标记（跳过 0xFF 填充字节）
func readJPEGMarker(r *bufio.Reader) (byte, error) {
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		if b != 0xFF {
			continue
		}
		// 标准允许多个 0xFF 填充，取首个非 0xFF 字节作为标记码
		for {
			next, err := r.ReadByte()
			if err != nil {
				return 0, err
			}
			if next == 0xFF {
				continue
			}
			return next, nil
		}
	}
}
