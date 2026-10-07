package qwentts

import (
	logger "LunarSubsystem/LoggerGeneral"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ==== DLL 动态加载 ====
// qwen3tts.dll 不再通过 cgo 导入库在进程启动时静态绑定，
// 而是运行时从模型目录加载（LOAD_WITH_ALTERED_SEARCH_PATH 使
// vulkan-1.dll / libgomp-1.dll / libwinpthread-1.dll 等依赖
// 自动从 qwen3tts.dll 同目录解析），引擎缺失时主程序可优雅降级。

// loadWithAlteredSearchPath 对应 LoadLibraryEx 的 LOAD_WITH_ALTERED_SEARCH_PATH 标志
const loadWithAlteredSearchPath = 0x0008

// ttsProcs 存储 qwen3tts.dll 导出函数地址（函数指针表）
var ttsProcs struct {
	// dll 为 qwen3tts.dll 的模块句柄
	dll windows.Handle
	// create 对应 qwen3_tts_create
	create uintptr
	// defaultParams 对应 qwen3_tts_default_params
	defaultParams uintptr
	// extractEmbeddingFile 对应 qwen3_tts_extract_embedding_file
	extractEmbeddingFile uintptr
	// synthesizeWithEmbedding 对应 qwen3_tts_synthesize_with_embedding
	synthesizeWithEmbedding uintptr
	// freeAudio 对应 qwen3_tts_free_audio
	freeAudio uintptr
	// getError 对应 qwen3_tts_get_error
	getError uintptr
}

// loadTTSLibrary 从 modelDir 加载 qwen3tts.dll 并解析导出函数地址
func loadTTSLibrary(modelDir string) error {
	dllPath := filepath.Join(modelDir, "qwen3tts.dll")
	if _, err := os.Stat(dllPath); err != nil {
		return fmt.Errorf("TTS 引擎库缺失: %s", dllPath)
	}

	// 转绝对路径，保证 LOAD_WITH_ALTERED_SEARCH_PATH 的依赖搜索基准正确
	absPath, err := filepath.Abs(dllPath)
	if err != nil {
		return fmt.Errorf("解析 TTS 引擎库路径失败: %w", err)
	}

	dll, err := windows.LoadLibraryEx(absPath, 0, loadWithAlteredSearchPath)
	if err != nil {
		return fmt.Errorf("加载 %s 失败: %w", absPath, err)
	}

	exports := []struct {
		name string
		dst  *uintptr
	}{
		{"qwen3_tts_create", &ttsProcs.create},
		{"qwen3_tts_default_params", &ttsProcs.defaultParams},
		{"qwen3_tts_extract_embedding_file", &ttsProcs.extractEmbeddingFile},
		{"qwen3_tts_synthesize_with_embedding", &ttsProcs.synthesizeWithEmbedding},
		{"qwen3_tts_free_audio", &ttsProcs.freeAudio},
		{"qwen3_tts_get_error", &ttsProcs.getError},
	}
	for _, e := range exports {
		proc, err := windows.GetProcAddress(dll, e.name)
		if err != nil {
			windows.FreeLibrary(dll)
			return fmt.Errorf("解析导出函数 %s 失败: %w", e.name, err)
		}
		*e.dst = proc
	}

	ttsProcs.dll = dll
	return nil
}

// cBytes 将 Go 字符串转为 NUL 结尾的 UTF-8 缓冲，供 C ABI 调用使用
func cBytes(s string) []byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return b
}

// cStringToGo 读取 C 侧返回的 NUL 结尾 UTF-8 字符串
func cStringToGo(s *byte) string {
	if s == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(s), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(s, n))
}

// ttsError 返回引擎最近一次错误信息
func ttsError(handle uintptr) string {
	if ttsProcs.getError == 0 {
		return ""
	}
	p, _, _ := syscall.SyscallN(ttsProcs.getError, handle)
	// 通过 Go 侧变量取址解引用 C 指针，避免 uintptr→unsafe.Pointer 的直接转换
	return cStringToGo(*(**byte)(unsafe.Pointer(&p)))
}

// cParams 与 qwen3tts_c_api.h 中 Qwen3TtsParams 的内存布局保持二进制一致
type cParams struct {
	// maxAudioTokens 最大音频 token 数
	maxAudioTokens int32
	// temperature 采样温度
	temperature float32
	// topP Top-P 采样
	topP float32
	// topK Top-K 采样
	topK int32
	// nThreads 线程数
	nThreads int32
	// repetitionPenalty 重复惩罚
	repetitionPenalty float32
	// languageID 语言 ID
	languageID int32
}

// cAudio 与 qwen3tts_c_api.h 中 Qwen3TtsAudio 的内存布局保持二进制一致
type cAudio struct {
	// samples 指向 PCM float32 单声道采样（C 侧内存，随 freeAudio 释放）
	samples *float32
	// nSamples 采样数
	nSamples int32
	// sampleRate 采样率（恒为 24000）
	sampleRate int32
}

// TTSEngine 用于存储TTS引擎实例
type TTSEngine struct {
	// handle 用于存储TTS引擎实例的句柄
	handle uintptr
	// modelDir 用于指定模型目录
	modelDir string
	// refAudio 用于指定参考音频文件
	refAudio string
	// mu 用于保护TTSEngine的互斥锁
	mu sync.Mutex
	// languageID 用于指定语言ID
	languageID int32
}

// speakerEmbedCache 用于存储语音特征的缓存
type speakerEmbedCache struct {
	// mu 用于保护speakerEmbedCache的互斥锁
	mu sync.Mutex
	// embeddings 用于存储语音特征的缓存
	embeddings map[string][]float32
	// fileHashes 用于存储参考音频文件的 SHA256 哈希，用于校验缓存一致性
	fileHashes map[string]string
	// cacheDir 用于指定缓存目录
	cacheDir string
}

// TTSRequest 用于存储TTS请求
type TTSRequest struct {
	// Text 用于存储要转换的文本
	Text string `json:"text"`
	// RefAudio 用于存储参考音频文件
	RefAudio string `json:"ref_audio,omitempty"`
	// LanguageID 用于存储语言ID
	LanguageID int32 `json:"language_id,omitempty"`
	// Temperature 用于控制生成随机性
	Temperature float32 `json:"temperature,omitempty"`
	// TopK 用于Top-K采样
	TopK int32 `json:"top_k,omitempty"`
	// TopP 用于Top-P采样
	TopP float32 `json:"top_p,omitempty"`
	// MaxTokens 用于设置最大生成token数
	MaxTokens int32 `json:"max_tokens,omitempty"`
	// RepetitionPenalty 用于控制重复惩罚
	RepetitionPenalty float32 `json:"repetition_penalty,omitempty"`
	// Threads 用于设置线程数
	Threads int32 `json:"threads,omitempty"`
	// DisableCache 用于禁用缓存映射机制
	DisableCache bool `json:"disable_cache,omitempty"`
}

// TTSResponse 用于存储TTS响应
type TTSResponse struct {
	// Success 用于存储是否成功
	Success bool `json:"success"`
	// Audio 用于存储音频文件路径
	Audio string `json:"audio,omitempty"`
	// Error 用于存储错误信息
	Error string `json:"error,omitempty"`
}

func (c *speakerEmbedCache) init() {
	os.MkdirAll(c.cacheDir, 0755)
	c.loadFromDisk()
}

func (c *speakerEmbedCache) loadFromDisk() {
	c.mu.Lock()
	defer c.mu.Unlock()

	files, err := os.ReadDir(c.cacheDir)
	if err != nil {
		logger.SubError("QWEN-TTS", "EmbedCache", "加载磁盘缓存失败: %v", err)
		return
	}

	loaded := 0
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".bin" {
			continue
		}

		filePath := filepath.Join(c.cacheDir, f.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.SubError("QWEN-TTS", "EmbedCache", "读取 %s 失败: %v", f.Name(), err)
			continue
		}

		// 最小有效大小：4(路径长度) + 4(embedding长度) = 8
		if len(data) < 8 {
			logger.SubError("QWEN-TTS", "EmbedCache", "跳过无效文件: %s (过小)", f.Name())
			continue
		}

		audioPathLen := int(data[0])<<24 | int(data[1])<<16 | int(data[2])<<8 | int(data[3])
		if 4+audioPathLen+4 > len(data) {
			logger.SubError("QWEN-TTS", "EmbedCache", "跳过损坏文件: %s (路径越界)", f.Name())
			continue
		}

		audioPath := string(data[4 : 4+audioPathLen])
		embedSize := int(data[4+audioPathLen])<<24 | int(data[4+audioPathLen+1])<<16 |
			int(data[4+audioPathLen+2])<<8 | int(data[4+audioPathLen+3])

		embedDataStart := 4 + audioPathLen + 4
		embedDataEnd := embedDataStart + embedSize*4
		if len(data) < embedDataEnd {
			logger.SubError("QWEN-TTS", "EmbedCache", "跳过大小不匹配文件: %s", f.Name())
			continue
		}

		embedData := data[embedDataStart:embedDataEnd]

		embedding := make([]float32, embedSize)
		for i := 0; i < embedSize; i++ {
			bits := uint32(embedData[i*4])<<24 | uint32(embedData[i*4+1])<<16 |
				uint32(embedData[i*4+2])<<8 | uint32(embedData[i*4+3])
			embedding[i] = *(*float32)(unsafe.Pointer(&bits))
		}

		// 读取文件哈希（4字节长度 + 哈希字符串）
		var fileHash string
		if len(data) >= embedDataEnd+4 {
			hashLenStart := embedDataEnd
			hashLen := int(data[hashLenStart])<<24 | int(data[hashLenStart+1])<<16 |
				int(data[hashLenStart+2])<<8 | int(data[hashLenStart+3])
			if len(data) >= hashLenStart+4+hashLen && hashLen > 0 {
				fileHash = string(data[hashLenStart+4 : hashLenStart+4+hashLen])
			}
		}

		// 以 fileHash 作为主键存入缓存，确保相同内容的音频在不同路径下也能命中
		if fileHash != "" {
			c.embeddings[fileHash] = embedding
			c.fileHashes[fileHash] = fileHash
			// 同时以路径为键建立映射，方便路径查找
			c.embeddings[audioPath] = embedding
			c.fileHashes[audioPath] = fileHash
		} else {
			// 旧格式无哈希：仅以路径为键
			c.embeddings[audioPath] = embedding
		}

		loaded++
	}

	if loaded > 0 {
		logger.SubInfo("QWEN-TTS", "EmbedCache", "从磁盘加载 %d 个 embedding", loaded)
	}
}

func (c *speakerEmbedCache) saveToDisk(audioPath string, embedding []float32) {
	c.mu.Lock()
	fileHash, hasHash := c.fileHashes[audioPath]
	c.mu.Unlock()

	if !hasHash || len(fileHash) < 16 {
		// 没有文件哈希时无法生成稳定的缓存文件名，跳过磁盘持久化
		logger.SubWarn("QWEN-TTS", "EmbedCache", "跳过保存（缺少文件哈希）: %s", audioPath)
		return
	}

	// 使用文件内容哈希前16位作为缓存文件名，相同内容的音频共享同一 .bin 文件
	fileName := fileHash[:16] + ".bin"
	filePath := filepath.Join(c.cacheDir, fileName)

	audioPathBytes := []byte(audioPath)
	hashBytes := []byte(fileHash)

	bufSize := 4 + len(audioPathBytes) + 4 + len(embedding)*4
	if hasHash {
		bufSize += 4 + len(hashBytes)
	}
	buf := make([]byte, bufSize)

	buf[0] = byte(len(audioPathBytes) >> 24)
	buf[1] = byte(len(audioPathBytes) >> 16)
	buf[2] = byte(len(audioPathBytes) >> 8)
	buf[3] = byte(len(audioPathBytes))
	copy(buf[4:], audioPathBytes)

	off := 4 + len(audioPathBytes)
	buf[off] = byte(len(embedding) >> 24)
	buf[off+1] = byte(len(embedding) >> 16)
	buf[off+2] = byte(len(embedding) >> 8)
	buf[off+3] = byte(len(embedding))

	off += 4
	for _, v := range embedding {
		bits := *(*uint32)(unsafe.Pointer(&v))
		buf[off] = byte(bits >> 24)
		buf[off+1] = byte(bits >> 16)
		buf[off+2] = byte(bits >> 8)
		buf[off+3] = byte(bits)
		off += 4
	}

	if hasHash {
		buf[off] = byte(len(hashBytes) >> 24)
		buf[off+1] = byte(len(hashBytes) >> 16)
		buf[off+2] = byte(len(hashBytes) >> 8)
		buf[off+3] = byte(len(hashBytes))
		copy(buf[off+4:], hashBytes)
	}

	if err := os.WriteFile(filePath, buf, 0644); err != nil {
		logger.SubError("QWEN-TTS", "EmbedCache", "保存 %s 失败: %v", fileName, err)
	}
}

func InitTTSEngine(modelDir, refAudio string) {
	ttsOnce.Do(func() {
		// 运行时从模型目录加载 qwen3tts.dll（依赖 DLL 自动从同目录解析）
		if err := loadTTSLibrary(modelDir); err != nil {
			logger.Error("QWEN-TTS", "%v", err)
			return
		}

		nThreads := max(4, runtime.NumCPU()/2)
		cModelDir := cBytes(modelDir)
		handle, _, _ := syscall.SyscallN(
			ttsProcs.create,
			uintptr(unsafe.Pointer(&cModelDir[0])),
			uintptr(int32(nThreads)),
		)
		if handle == 0 {
			logger.Error("QWEN-TTS", "引擎初始化失败，模型目录: %s", modelDir)
			return
		}

		globalTTS = &TTSEngine{
			handle:     handle,
			modelDir:   modelDir,
			refAudio:   refAudio,
			languageID: 2055,
		}

		embedCache.init()

		logger.Info("QWEN-TTS", "引擎初始化成功")
	})
}

func computeFileHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

func getOrExtractEmbedding(refAudio string) ([]float32, error) {
	currentHash := computeFileHash(refAudio)
	if currentHash == "" {
		return nil, fmt.Errorf("无法读取参考音频文件: %s", refAudio)
	}

	embedCache.mu.Lock()
	// 优先以文件内容哈希查找缓存，确保相同内容的音频在不同路径下也能命中
	cachedEmbedding, found := embedCache.embeddings[currentHash]
	if !found {
		// 回退到路径查找
		cachedEmbedding, found = embedCache.embeddings[refAudio]
	}

	if found {
		// 验证哈希一致性
		cachedHash, hasHash := embedCache.fileHashes[refAudio]
		if !hasHash {
			cachedHash, hasHash = embedCache.fileHashes[currentHash]
		}
		if hasHash && cachedHash == currentHash {
			embedCache.mu.Unlock()
			logger.SubInfo("QWEN-TTS", "EmbedCache", "命中缓存: %s (hash=%s)", refAudio, currentHash[:16])
			return cachedEmbedding, nil
		}
		// 哈希不匹配说明文件已变更，需要重新提取
	}
	embedCache.mu.Unlock()

	cRefAudio := cBytes(refAudio)

	const maxEmbedSize = 2048
	embedBuf := make([]float32, maxEmbedSize)

	logger.SubInfo("QWEN-TTS", "EmbedCache", "正在提取 speaker embedding: %s", refAudio)
	embedSize, _, _ := syscall.SyscallN(
		ttsProcs.extractEmbeddingFile,
		globalTTS.handle,
		uintptr(unsafe.Pointer(&cRefAudio[0])),
		uintptr(unsafe.Pointer(&embedBuf[0])),
		uintptr(int32(maxEmbedSize)),
	)

	if int32(embedSize) <= 0 {
		errStr := ttsError(globalTTS.handle)
		return nil, fmt.Errorf("提取 speaker embedding失败: %s", errStr)
	}

	embedding := make([]float32, embedSize)
	copy(embedding, embedBuf[:embedSize])

	embedCache.mu.Lock()
	// 以路径和哈希双重索引存储，确保重启后通过任一键都能命中
	embedCache.embeddings[refAudio] = embedding
	embedCache.embeddings[currentHash] = embedding
	embedCache.fileHashes[refAudio] = currentHash
	embedCache.fileHashes[currentHash] = currentHash
	embedCache.mu.Unlock()

	embedCache.saveToDisk(refAudio, embedding)

	logger.SubInfo("QWEN-TTS", "EmbedCache", "已缓存参考音频 %s 的 embedding (embedding大小=%d, hash=%s)", refAudio, embedSize, currentHash[:16])
	return embedding, nil
}

func SynthesizeText(text, refAudio string, languageID int32, temperature float32, topK int32, topP float32, maxTokens int32, repetitionPenalty float32, threads int32) ([]float32, error) {
	if globalTTS == nil || globalTTS.handle == 0 {
		return nil, fmt.Errorf("TTS 引擎未初始化")
	}

	if refAudio == "" {
		refAudio = globalTTS.refAudio
	}
	if languageID == 0 {
		languageID = globalTTS.languageID
	}

	globalTTS.mu.Lock()
	defer globalTTS.mu.Unlock()

	embedding, err := getOrExtractEmbedding(refAudio)
	if err != nil {
		return nil, err
	}

	cText := cBytes(text)

	var params cParams
	syscall.SyscallN(ttsProcs.defaultParams, uintptr(unsafe.Pointer(&params)))
	if threads <= 0 {
		threads = int32(max(1, runtime.NumCPU()-1))
	}
	params.nThreads = threads
	params.languageID = languageID
	if temperature != 0 {
		params.temperature = temperature
	}
	if topK != 0 {
		params.topK = topK
	}
	if topP != 0 {
		params.topP = topP
	}
	if maxTokens != 0 {
		params.maxAudioTokens = maxTokens
	}
	if repetitionPenalty != 0 {
		params.repetitionPenalty = repetitionPenalty
	}

	result, _, _ := syscall.SyscallN(
		ttsProcs.synthesizeWithEmbedding,
		globalTTS.handle,
		uintptr(unsafe.Pointer(&cText[0])),
		uintptr(unsafe.Pointer(&embedding[0])),
		uintptr(int32(len(embedding))),
		uintptr(unsafe.Pointer(&params)),
	)

	if result == 0 {
		errStr := ttsError(globalTTS.handle)
		return nil, fmt.Errorf("TTS 合成失败: %s", errStr)
	}

	defer syscall.SyscallN(ttsProcs.freeAudio, result)

	// 通过 Go 侧变量取址解引用 C 结构体指针，避免 uintptr→unsafe.Pointer 的直接转换
	audio := *(**cAudio)(unsafe.Pointer(&result))
	nSamples := int(audio.nSamples)
	if nSamples == 0 {
		return nil, fmt.Errorf("TTS 合成结果为空")
	}

	samples := make([]float32, nSamples)
	copy(samples, unsafe.Slice(audio.samples, nSamples))

	return samples, nil
}

func EncodePCMToWAV(samples []float32, sampleRate int) []byte {
	numSamples := len(samples)
	byteRate := sampleRate * 2
	blockAlign := 2
	dataSize := numSamples * 2
	fileSize := 36 + dataSize

	buf := make([]byte, 44+dataSize)

	buf[0] = 'R'
	buf[1] = 'I'
	buf[2] = 'F'
	buf[3] = 'F'
	putUint32LE(buf[4:], uint32(fileSize))
	buf[8] = 'W'
	buf[9] = 'A'
	buf[10] = 'V'
	buf[11] = 'E'
	buf[12] = 'f'
	buf[13] = 'm'
	buf[14] = 't'
	buf[15] = ' '
	putUint32LE(buf[16:], 16)
	putUint16LE(buf[20:], 1)
	putUint16LE(buf[22:], 1)
	putUint32LE(buf[24:], uint32(sampleRate))
	putUint32LE(buf[28:], uint32(byteRate))
	putUint16LE(buf[32:], uint16(blockAlign))
	putUint16LE(buf[34:], 16)
	buf[36] = 'd'
	buf[37] = 'a'
	buf[38] = 't'
	buf[39] = 'a'
	putUint32LE(buf[40:], uint32(dataSize))

	for i, sample := range samples {
		val := int16(max(-32768, min(32767, int32(sample*32767.0))))
		offset := 44 + i*2
		buf[offset] = byte(val)
		buf[offset+1] = byte(val >> 8)
	}

	return buf
}

func putUint16LE(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func putUint32LE(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

// synthesizeWithCache 执行带缓存支持的TTS合成
// 当 DisableCache 为 false 时优先查询缓存，缓存未命中时执行实际合成并写入缓存
// 使用 singleflight 模式防止高并发下的缓存击穿
func synthesizeWithCache(req *TTSRequest) (string, error) {
	cacheKey := buildCacheKey(req)

	// 如果未禁用缓存，尝试从缓存获取
	if !req.DisableCache {
		if audioData, hit := ttsCache.Get(cacheKey); hit {
			logger.SubInfo("QWEN-TTS", "Cache", "缓存命中，返回已缓存音频: [%s]", req.Text)
			return audioData, nil
		}
	}

	// 单飞模式：防止并发重复合成（缓存击穿保护）
	inflight, exists := ttsCache.GetOrCreateInflight(cacheKey)
	if exists {
		logger.SubInfo("QWEN-TTS", "Cache", "等待已有合成完成: [%s]", req.Text)
		audioData, err := inflight.Wait()
		if err != nil {
			return "", err
		}
		return audioData, nil
	}
	defer ttsCache.RemoveInflight(cacheKey)

	// 执行实际的TTS合成
	samples, err := synthFunc(req.Text, req.RefAudio, req.LanguageID, req.Temperature, req.TopK, req.TopP, req.MaxTokens, req.RepetitionPenalty, req.Threads)
	if err != nil {
		inflight.Complete("", err)
		return "", err
	}

	wavData := EncodePCMToWAV(samples, 24000)
	audioBase64 := base64.StdEncoding.EncodeToString(wavData)

	// 无论 disable_cache 是否为 true，都更新缓存
	// 当 disable_cache=true 时，强制生成新数据并更新缓存
	ttsCache.Set(cacheKey, audioBase64)
	inflight.Complete(audioBase64, nil)

	logger.Info("QWEN-TTS", "合成完成: [%s] 采样数: %d", req.Text, len(samples))
	return audioBase64, nil
}

func TTSHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TTSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error("QWEN-TTS", "解析TTS请求失败: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TTSResponse{
			Success: false,
			Error:   "无效的请求格式",
		})
		return
	}

	if req.Text == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TTSResponse{
			Success: false,
			Error:   "文本内容不能为空",
		})
		return
	}

	audioBase64, err := synthesizeWithCache(&req)
	if err != nil {
		logger.Error("QWEN-TTS", "合成失败: [%s] %v", req.Text, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TTSResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(TTSResponse{
		Success: true,
		Audio:   audioBase64,
	})
}

func UploadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.ParseMultipartForm(32 << 20)

	file, header, err := r.FormFile("audio")
	if err != nil {
		logger.Error("QWEN-TTS", "解析上传文件失败: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "无法获取上传文件",
		})
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".wav"
	}

	uploadDir := "./local_data/audios"
	os.MkdirAll(uploadDir, 0755)

	tempPath := filepath.Join(uploadDir, "ref_"+fmt.Sprintf("%d_%d", os.Getpid(), time.Now().UnixNano())+ext)

	outFile, err := os.Create(tempPath)
	if err != nil {
		logger.Error("QWEN-TTS", "创建文件失败: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "保存文件失败",
		})
		return
	}
	defer outFile.Close()

	buf := make([]byte, 32*1024)
	for {
		n, _ := file.Read(buf)
		if n == 0 {
			break
		}
		outFile.Write(buf[:n])
	}

	absPath, _ := filepath.Abs(tempPath)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"path":    absPath,
		"name":    header.Filename,
	})
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"service": "simple-tts",
		"port":    36365,
	})
}
