package handlers

import (
	"config"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ==== 类型定义 ====

// MusicRenderRequest 音乐渲染请求
type MusicRenderRequest struct {
	ABCNotation string `json:"abc_notation"` // ABC 记谱法乐谱文本
	Title       string `json:"title"`        // 作品名称（用于文件命名）
}

// MusicRenderResponse 音乐渲染响应
type MusicRenderResponse struct {
	Success  bool   `json:"success"`
	AudioURL string `json:"audio_url,omitempty"` // WAV 音频文件 URL
	MidiURL  string `json:"midi_url,omitempty"`  // MIDI 文件 URL（可选）
	Duration string `json:"duration,omitempty"`  // 渲染耗时
	Error    string `json:"error,omitempty"`     // 错误信息
	FileName string `json:"file_name,omitempty"` // 文件名
}

// ==== 可执行文件路径 ====

// getFluidSynthPath 获取 FluidSynth 可执行文件路径
func getFluidSynthPath() string {
	// 项目规范路径：local_data/package/fluidsynth/
	localPath := filepath.Join(*config.LocalDir, "package", "fluidsynth", "fluidsynth.exe")
	// 转为绝对路径，确保 exec.Command 能正确解析（相对路径在 CreateProcess 时可能失败）
	if absPath, err := filepath.Abs(localPath); err == nil {
		localPath = absPath
	}
	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}
	// 尝试 PATH 中的 fluidsynth
	if p, err := exec.LookPath("fluidsynth"); err == nil {
		return p
	}
	return ""
}

// getAbc2midiPath 获取 abc2midi 可执行文件路径
func getAbc2midiPath() string {
	localPath := filepath.Join(*config.LocalDir, "package", "fluidsynth", "abc2midi.exe")
	if absPath, err := filepath.Abs(localPath); err == nil {
		localPath = absPath
	}
	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}
	if p, err := exec.LookPath("abc2midi"); err == nil {
		return p
	}
	return ""
}

// getSoundFontPath 获取 SoundFont 文件路径
func getSoundFontPath() string {
	// 项目规范路径：local_data/package/soundfonts/
	sfDir := filepath.Join(*config.LocalDir, "package", "soundfonts")
	if absDir, err := filepath.Abs(sfDir); err == nil {
		sfDir = absDir
	}
	candidates := []string{
		filepath.Join(sfDir, "general.sf2"),
		filepath.Join(sfDir, "GeneralUser.sf2"),
		filepath.Join(sfDir, "FluidR3_GM.sf2"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// 搜索目录下任意 .sf2 文件
	if entries, err := os.ReadDir(sfDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".sf2") {
				return filepath.Join(sfDir, entry.Name())
			}
		}
	}
	return ""
}

// getOutputDir 获取音频输出目录
func getOutputDir() string {
	// 项目规范路径：local_data/audios/
	outputDir := filepath.Join(*config.LocalDir, "audios")
	if absDir, err := filepath.Abs(outputDir); err == nil {
		outputDir = absDir
	}
	os.MkdirAll(outputDir, 0755)
	return outputDir
}

// CheckMusicDeps 检查音乐渲染依赖是否就绪
func CheckMusicDeps() (fluidSynthOK bool, soundFontOK bool, abc2midiOK bool) {
	fluidSynthOK = getFluidSynthPath() != ""
	soundFontOK = getSoundFontPath() != ""
	abc2midiOK = getAbc2midiPath() != ""
	return
}

// ==== 核心渲染管线 ====

// RenderMusicInternal 内部调用：ABC→MIDI→WAV 渲染管线
// 供 Go 后端直接调用（无需 HTTP 请求），由 adapters 包的 pushContext 拦截 'music' 类型时触发
func RenderMusicInternal(abcNotation string, title string) (audioURL string, fileName string, err error) {
	startTime := time.Now()

	// 检查依赖
	fluidSynthPath := getFluidSynthPath()
	if fluidSynthPath == "" {
		return "", "", fmt.Errorf("FluidSynth 未安装")
	}
	soundFontPath := getSoundFontPath()
	if soundFontPath == "" {
		return "", "", fmt.Errorf("SoundFont 未找到")
	}

	// 生成唯一文件名
	timestamp := time.Now().Format("20060102_150405")
	safeTitle := sanitizeFileName(title)
	if safeTitle == "" {
		safeTitle = "music"
	}
	baseName := fmt.Sprintf("%s_%s", safeTitle, timestamp)

	outputDir := getOutputDir()
	abcPath := filepath.Join(outputDir, baseName+".abc")
	midiPath := filepath.Join(outputDir, baseName+".mid")
	wavPath := filepath.Join(outputDir, baseName+".wav")

	// 步骤1：写入 ABC 文件
	if err := os.WriteFile(abcPath, []byte(abcNotation), 0644); err != nil {
		return "", "", fmt.Errorf("ABC 文件写入失败: %w", err)
	}

	// 步骤2：ABC → MIDI
	abc2midiPath := getAbc2midiPath()
	if abc2midiPath != "" {
		cmd := exec.Command(abc2midiPath, abcPath, "-o", midiPath)
		cmd.Dir = outputDir
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[音乐渲染] abc2midi 失败: %v, 输出: %s", err, string(output))
		} else {
			log.Printf("[音乐渲染] abc2midi 成功: %s", midiPath)
		}
	} else {
		log.Printf("[音乐渲染] abc2midi 未安装，使用内置转换器")
		if err := convertABCToMIDI(abcPath, midiPath); err != nil {
			cleanupTempFiles(abcPath)
			return "", "", fmt.Errorf("ABC 转 MIDI 失败: %w", err)
		}
	}

	// 验证 MIDI 文件存在
	if _, statErr := os.Stat(midiPath); os.IsNotExist(statErr) {
		cleanupTempFiles(abcPath)
		return "", "", fmt.Errorf("MIDI 文件生成失败")
	}

	// 步骤3：MIDI → WAV（使用 FluidSynth）
	cmd := exec.Command(fluidSynthPath,
		"-ni",
		"-F", wavPath,
		"-r", "44100",
		"-o", "synth.reverb.active=1",
		"-o", "synth.chorus.active=1",
		"-o", "synth.polyphony=256",
		soundFontPath,
		midiPath,
	)
	cmd.Dir = outputDir
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("[音乐渲染] FluidSynth 失败: %v, 输出: %s", err, string(output))
		cleanupTempFiles(abcPath, midiPath)
		return "", "", fmt.Errorf("音频渲染失败: %w", err)
	}

	// 验证 WAV 文件存在
	if _, statErr := os.Stat(wavPath); os.IsNotExist(statErr) {
		cleanupTempFiles(abcPath, midiPath)
		return "", "", fmt.Errorf("WAV 文件生成失败")
	}

	// 清理中间文件
	cleanupTempFiles(abcPath, midiPath)

	duration := time.Since(startTime)
	log.Printf("[音乐渲染] 渲染完成: %s, 耗时: %v", wavPath, duration)

	// 构建音频 URL
	wavRelPath := strings.Replace(wavPath, filepath.ToSlash(*config.LocalDir)+"/", "", 1)
	wavRelPath = filepath.ToSlash(wavRelPath)
	audioURL = "/file/read/" + wavRelPath
	fileName = baseName + ".wav"

	return audioURL, fileName, nil
}

// ==== HTTP 处理器 ====

// MusicRenderHandler 处理音乐渲染请求
// POST /music/render
func MusicRenderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendMusicError(w, "请求体读取失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req MusicRenderRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendMusicError(w, "请求格式错误", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.ABCNotation) == "" {
		sendMusicError(w, "ABC 乐谱为空", http.StatusBadRequest)
		return
	}

	audioURL, fileName, renderErr := RenderMusicInternal(req.ABCNotation, req.Title)
	if renderErr != nil {
		code := http.StatusInternalServerError
		if strings.Contains(renderErr.Error(), "未安装") || strings.Contains(renderErr.Error(), "未找到") {
			code = http.StatusServiceUnavailable
		}
		sendMusicError(w, renderErr.Error(), code)
		return
	}

	resp := MusicRenderResponse{
		Success:  true,
		AudioURL: audioURL,
		FileName: fileName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// MusicDepsHandler 返回音乐渲染依赖状态
// GET /music/deps
func MusicDepsHandler(w http.ResponseWriter, r *http.Request) {
	fluidOK, sfOK, abcOK := CheckMusicDeps()
	resp := map[string]bool{
		"fluidsynth": fluidOK,
		"soundfont":  sfOK,
		"abc2midi":   abcOK,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ==== 辅助函数 ====

func sendMusicError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(MusicRenderResponse{
		Success: false,
		Error:   msg,
	})
}

func sanitizeFileName(name string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, name)
	if len(safe) > 50 {
		safe = safe[:50]
	}
	return safe
}

func cleanupTempFiles(paths ...string) {
	for _, p := range paths {
		os.Remove(p)
	}
}
