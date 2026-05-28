package generate

import (
	"sd_lunar/engine"
	"sd_lunar/mempool"
)

type Params struct {
	Prompt         string
	NegativePrompt string
	Width          int
	Height         int
	Steps          int
	Sampler        string
	Scheduler      string
	CFGScale       float32
	Seed           int64
	ClipSkip       int
}

type Img2ImgParams struct {
	Params
	Strength     float32
	RefImageData []byte
	RefWidth     uint32
	RefHeight    uint32
	RefChannels  uint32
}

type Result struct {
	Width   uint32
	Height  uint32
	Channel uint32
	Data    []byte
	Seed    int64
}

func TextToImage(p Params) (*Result, error) {
	eng := engine.GetEngine()
	if eng == nil || !eng.IsReady() {
		return nil, engine.ErrNotReady
	}

	cfg := engine.GenerationConfig{
		Prompt:         p.Prompt,
		NegativePrompt: p.NegativePrompt,
		Width:          p.Width,
		Height:         p.Height,
		Steps:          p.Steps,
		Sampler:        p.Sampler,
		Scheduler:      p.Scheduler,
		CFGScale:       p.CFGScale,
		Seed:           p.Seed,
		ClipSkip:       p.ClipSkip,
		BatchCount:     1,
	}

	img, err := eng.GenerateTextToImage(cfg)
	if err != nil {
		return nil, err
	}

	result := &Result{
		Width:   img.Width,
		Height:  img.Height,
		Channel: img.Channel,
		Seed:    img.Seed,
	}

	bufPool := mempool.GetImageBufferPool()
	result.Data = bufPool.Get(len(img.Data))
	copy(result.Data, img.Data)

	return result, nil
}

func ImageToImage(p Img2ImgParams) (*Result, error) {
	eng := engine.GetEngine()
	if eng == nil || !eng.IsReady() {
		return nil, engine.ErrNotReady
	}

	cfg := engine.GenerationConfig{
		Prompt:         p.Prompt,
		NegativePrompt: p.NegativePrompt,
		Width:          p.Width,
		Height:         p.Height,
		Steps:          p.Steps,
		Sampler:        p.Sampler,
		Scheduler:      p.Scheduler,
		CFGScale:       p.CFGScale,
		Seed:           p.Seed,
		ClipSkip:       p.ClipSkip,
		Strength:       p.Strength,
		BatchCount:     1,
	}

	img, err := eng.GenerateImageToImage(cfg, p.RefImageData, p.RefWidth, p.RefHeight, p.RefChannels)
	if err != nil {
		return nil, err
	}

	result := &Result{
		Width:   img.Width,
		Height:  img.Height,
		Channel: img.Channel,
		Seed:    img.Seed,
	}

	bufPool := mempool.GetImageBufferPool()
	result.Data = bufPool.Get(len(img.Data))
	copy(result.Data, img.Data)

	return result, nil
}
