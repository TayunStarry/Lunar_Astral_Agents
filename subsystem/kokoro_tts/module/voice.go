package module

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadVoices 加载 voices 目录下全部 .bin 音色文件
func LoadVoices(voicesDir string) (map[string]*Voice, []string, error) {
	entries, err := os.ReadDir(voicesDir)
	if err != nil {
		return nil, nil, fmt.Errorf("读取音色目录失败: %w", err)
	}
	voices := make(map[string]*Voice)
	var order []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".bin") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".bin")
		rows, err := loadVoiceRows(filepath.Join(voicesDir, e.Name()))
		if err != nil {
			return nil, nil, fmt.Errorf("加载音色 %s 失败: %w", name, err)
		}
		voices[name] = &Voice{Name: name, Rows: rows}
		order = append(order, name)
	}
	sort.Strings(order)
	if len(voices) == 0 {
		return nil, nil, fmt.Errorf("音色目录为空: %s", voicesDir)
	}
	return voices, order, nil
}

// loadVoiceRows 读取单个音色 .bin 文件为 [510, 256] float32 行数据
func loadVoiceRows(path string) ([][]float32, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	expect := VoiceRowCount * StyleDim * 4
	if len(data) != expect {
		return nil, fmt.Errorf("音色文件大小异常: 实际 %d 字节, 期望 %d", len(data), expect)
	}
	rows := make([][]float32, VoiceRowCount)
	for i := 0; i < VoiceRowCount; i++ {
		row := make([]float32, StyleDim)
		base := i * StyleDim * 4
		for j := 0; j < StyleDim; j++ {
			bits := binary.LittleEndian.Uint32(data[base+j*4:])
			row[j] = math.Float32frombits(bits)
		}
		rows[i] = row
	}
	return rows, nil
}

// styleFor 按音素数量选择音色行（n 个音素取第 n-1 行，越界时截断）
func styleFor(v *Voice, length int) []float32 {
	idx := length - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(v.Rows) {
		idx = len(v.Rows) - 1
	}
	return v.Rows[idx]
}
