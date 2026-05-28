package mempool

import (
	"sync"
)

const (
	maxBufferSize = 128 * 1024 * 1024
)

type BufferPool struct {
	pool sync.Pool
}

var (
	imageBufferPool *BufferPool
	poolOnce        sync.Once
)

func InitImagePool() {
	poolOnce.Do(func() {
		imageBufferPool = &BufferPool{
			pool: sync.Pool{
				New: func() interface{} {
					buf := make([]byte, 0, 64*1024)
					return &buf
				},
			},
		}
	})
}

func GetImageBufferPool() *BufferPool {
	if imageBufferPool == nil {
		InitImagePool()
	}
	return imageBufferPool
}

func (p *BufferPool) Get(minSize int) []byte {
	bufPtr := p.pool.Get().(*[]byte)
	buf := *bufPtr
	if cap(buf) < minSize {
		if minSize > maxBufferSize {
			minSize = maxBufferSize
		}
		buf = make([]byte, minSize)
	} else {
		buf = buf[:minSize]
	}
	return buf
}

func (p *BufferPool) Put(buf []byte) {
	if cap(buf) > maxBufferSize {
		return
	}
	buf = buf[:0]
	p.pool.Put(&buf)
}

type ImagePool struct {
	pool sync.Pool
}

var (
	imageObjectPool *ImagePool
	objPoolOnce     sync.Once
)

type PooledImage struct {
	Width   uint32
	Height  uint32
	Channel uint32
	Data    []byte
}

func InitObjectPool() {
	objPoolOnce.Do(func() {
		imageObjectPool = &ImagePool{
			pool: sync.Pool{
				New: func() interface{} {
					return &PooledImage{}
				},
			},
		}
	})
}

func GetImageObjectPool() *ImagePool {
	if imageObjectPool == nil {
		InitObjectPool()
	}
	return imageObjectPool
}

func (p *ImagePool) Get() *PooledImage {
	return p.pool.Get().(*PooledImage)
}

func (p *ImagePool) Put(img *PooledImage) {
	img.Width = 0
	img.Height = 0
	img.Channel = 0
	img.Data = nil
	p.pool.Put(img)
}

func Reset() {
	imageBufferPool = nil
	imageObjectPool = nil
	poolOnce = sync.Once{}
	objPoolOnce = sync.Once{}
}
