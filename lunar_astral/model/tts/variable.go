package tts

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"unsafe"
)

// maxCacheSize 是缓存的最大大小，用于限制缓存中存储的语音数量。
const maxCacheSize = 15

// ttsCache 是一个缓存，用于存储已生成的语音。
var ttsCache = &TTSCache{
	items: make(map[string]*cacheEntry),
	order: make([]string, 0, maxCacheSize),
}

// ttsOnce 是一个一次初始化的同步机制，用于确保 TTS 引擎只被初始化一次。
var ttsOnce sync.Once

// globalTTS 是全局的 TTS 引擎实例，用于生成语音。
var globalTTS *TTSEngine

// streamCacheMu 是一个互斥锁，用于保护流式缓存的并发访问。
var streamCacheMu sync.Mutex

// streamCacheItems 是一个映射，用于存储流式缓存项。
var streamCacheItems = make(map[string]*streamCacheEntry)

// embedCache 是全局的 speaker embedding 缓存实例。
var embedCache = &speakerEmbedCache{
	embeddings: make(map[string][]float32),
	cacheDir:   "./local_data/embed_cache",
}

func initEmbedCache() {
	os.MkdirAll(embedCache.cacheDir, 0755)
	embedCache.loadFromDisk()
}

func (c *speakerEmbedCache) loadFromDisk() {
	c.mu.Lock()
	defer c.mu.Unlock()

	files, err := os.ReadDir(c.cacheDir)
	if err != nil {
		log.Printf("[EmbedCache] 加载磁盘缓存失败: %v", err)
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
			log.Printf("[EmbedCache] 读取 %s 失败: %v", f.Name(), err)
			continue
		}

		if len(data) < 8 || len(data)%4 != 0 {
			log.Printf("[EmbedCache] 跳过无效文件: %s", f.Name())
			continue
		}

		audioPathLen := int(data[0])<<24 | int(data[1])<<16 | int(data[2])<<8 | int(data[3])
		if 4+audioPathLen+4 > len(data) {
			log.Printf("[EmbedCache] 跳过损坏文件: %s", f.Name())
			continue
		}

		audioPath := string(data[4 : 4+audioPathLen])
		embedSize := int(data[4+audioPathLen])<<24 | int(data[4+audioPathLen+1])<<16 |
			int(data[4+audioPathLen+2])<<8 | int(data[4+audioPathLen+3])

		embedData := data[4+audioPathLen+4:]
		if len(embedData) != embedSize*4 {
			log.Printf("[EmbedCache] 跳过大小不匹配文件: %s", f.Name())
			continue
		}

		embedding := make([]float32, embedSize)
		for i := 0; i < embedSize; i++ {
			bits := uint32(embedData[i*4])<<24 | uint32(embedData[i*4+1])<<16 |
				uint32(embedData[i*4+2])<<8 | uint32(embedData[i*4+3])
			embedding[i] = *(*float32)(unsafe.Pointer(&bits))
		}

		c.embeddings[audioPath] = embedding
		loaded++
	}

	if loaded > 0 {
		log.Printf("[EmbedCache] 从磁盘加载 %d 个 embedding", loaded)
	}
}

func (c *speakerEmbedCache) saveToDisk(audioPath string, embedding []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hash := fmt.Sprintf("%x", []byte(audioPath))
	if len(hash) > 16 {
		hash = hash[:16]
	}
	fileName := hash + ".bin"
	filePath := filepath.Join(c.cacheDir, fileName)

	audioPathBytes := []byte(audioPath)
	buf := make([]byte, 4+len(audioPathBytes)+4+len(embedding)*4)

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

	if err := os.WriteFile(filePath, buf, 0644); err != nil {
		log.Printf("[EmbedCache] 保存 %s 失败: %v", fileName, err)
	}
}
