package BilibiliLife

// audit.go 视频内容审核：在切片理解前检测静态/高度雷同画面。
//
// 背景：部分视频存在十几秒以上的静态画面或高度雷同的连续帧，这类输入极易触发
// 多模态模型的「无限思考循环」（同一视频的其他切片也有很高的连带触发概率）。
// 因此在下载完成、理解开始前对整段视频做一次帧差检测：命中即舍弃该视频——
// 计入已看记录（不占观看额度，VideoIndex 不增加），回到选片重新挑。
//
// 检测算法：每 2 秒采样一帧缩为 16x16 灰度网格，计算相邻帧平均绝对差；
// 连续 ≥stasisMinRun 个间隔（约十几秒）差异都低于阈值即判定为静态/高度雷同。
// 一个 60 秒切片里 10 秒的静止就是很长的片段，所以滑窗长度按 10 秒设定。

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	ffmpeg "github.com/u2takey/ffmpeg-go"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

const (
	// stasisSampleInterval 采样间隔（秒）：每 2 秒一帧足够捕捉「十几秒的静止」
	stasisSampleInterval = 2.0
	// stasisDiffThreshold 相邻帧平均绝对差阈值（0-255 灰度尺度）：
	// 低于该值视为「画面几乎没变」（静态或高度雷同）
	stasisDiffThreshold = 4.0
	// stasisMinRun 连续低差异间隔数：5 个间隔 × 2 秒 ≈ 10 秒的静止即触发舍弃
	stasisMinRun = 5
	// stasisMaxSamples 采样帧数上限（600 秒视频 @2s/帧 = 300 帧）
	stasisMaxSamples = 300
	// auditRejectQuotaFactor 单轮允许的审核失败次数相对观看额度的倍数：
	// 放弃阈值 = 本轮视频观看额度 × 该倍数（额度 3 → 连续 9 个视频未通过才放弃），
	// 避免审核器与推荐池互相拉扯造成无限选片循环，也避免一两部片子就中止一轮。
	auditRejectQuotaFactor = 3
)

// auditRejectLimit 本轮允许的静态画面审核失败次数（观看额度 × 3）。
// 额度非法（<=0）时按最小额度兜底，避免 0 阈值让本轮一开场就被放弃。
func auditRejectLimit(videosPerRound int) int {
	if videosPerRound < videoMinCount {
		videosPerRound = videoMinCount
	}
	return videosPerRound * auditRejectQuotaFactor
}

// auditVideoStasis 检测视频是否存在静态/高度雷同的连续画面段。
//
// 返回 rejected=true 表示应舍弃该视频。审核器自身故障（抽帧失败等）不阻断生活链：
// 返回 rejected=false 并记录告警，宁可放行也不让 ffmpeg 偶发问题杀掉整轮。
func auditVideoStasis(localPath string) (rejected bool, err error) {
	grids, err := sampleGrayGrids(localPath)
	if err != nil {
		return false, fmt.Errorf("审核抽帧失败: %w", err)
	}
	if len(grids) < stasisMinRun+1 {
		// 帧数不足以判定（视频太短或抽帧过少），放行
		return false, nil
	}
	rejected = hasStasisRun(grids)
	if rejected {
		logger.Warn("BilibiliLife", "视频存在十几秒以上的静态/高度雷同画面，判定为高风险输入: %s", filepath.Base(localPath))
	}
	return rejected, nil
}

// sampleGrayGrids 用 ffmpeg 等间隔采样帧并缩为 16x16 灰度网格。
func sampleGrayGrids(localPath string) ([][]uint8, error) {
	tmpDir, err := os.MkdirTemp("", "bili_life_audit_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	pattern := filepath.Join(tmpDir, "f-%04d.png")
	args := ffmpeg.KwArgs{
		"vf":       fmt.Sprintf("fps=%.2f,scale=16:16,format=gray", 1.0/stasisSampleInterval),
		"frames:v": strconv.Itoa(stasisMaxSamples),
		"f":        "image2",
		"loglevel": "error",
	}
	stream := ffmpeg.Input(localPath).Output(pattern, args)
	if config.FfmpegPath != "" {
		stream = stream.SetFfmpegPath(config.FfmpegPath)
	}
	if err := stream.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg 采样帧失败: %w", err)
	}
	files, err := filepath.Glob(filepath.Join(tmpDir, "f-*.png"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	grids := make([][]uint8, 0, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			continue
		}
		grids = append(grids, grayGrid(img))
	}
	return grids, nil
}

// grayGrid 把任意图像规整为 16x16 的灰度值网格（256 个 0-255 值）。
func grayGrid(img image.Image) []uint8 {
	const side = 16
	bounds := img.Bounds()
	grid := make([]uint8, side*side)
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			// 双线性近似：直接取比例映射位置的像素（16x16 的粗网格不需要精确插值）
			sx := bounds.Min.X + (x*(bounds.Dx())+side/2)/side
			sy := bounds.Min.Y + (y*(bounds.Dy())+side/2)/side
			r, g, b, _ := img.At(sx, sy).RGBA()
			luma := (299*r + 587*g + 114*b) / 1000 / 257
			grid[y*side+x] = uint8(math.Min(float64(luma), 255))
		}
	}
	return grid
}

// meanAbsDiff 计算两个灰度网格的平均绝对差（0-255 尺度）
func meanAbsDiff(a, b []uint8) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 255
	}
	sum := 0
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		sum += d
	}
	return float64(sum) / float64(len(a))
}

// hasStasisRun 滑窗判定：连续 ≥stasisMinRun 个相邻间隔差异都低于阈值即为命中。
func hasStasisRun(grids [][]uint8) bool {
	run := 0
	for i := 1; i < len(grids); i++ {
		if meanAbsDiff(grids[i-1], grids[i]) < stasisDiffThreshold {
			run++
			if run >= stasisMinRun {
				return true
			}
		} else {
			run = 0
		}
	}
	return false
}
