package BilibiliLife

import (
	"image"
	"image/color"
	"testing"
)

// solidGrid 生成单色 16x16 灰度网格
func solidGrid(v uint8) []uint8 {
	grid := make([]uint8, 256)
	for i := range grid {
		grid[i] = v
	}
	return grid
}

// TestHasStasisRunStaticVideo 钉住静态/雷同画面的滑窗判定。
func TestHasStasisRunStaticVideo(t *testing.T) {
	// 全程静止：应命中
	static := make([][]uint8, 20)
	for i := range static {
		static[i] = solidGrid(128)
	}
	if !hasStasisRun(static) {
		t.Fatal("全程静止的画面应被判定为静态")
	}

	// 中间有一段 10 秒（5 个间隔）静止：应命中
	grids := make([][]uint8, 20)
	for i := range grids {
		grids[i] = solidGrid(uint8(30 + i*10)) // 每对相邻差 10，不触发
	}
	for i := 6; i < 12; i++ { // 5 个连续间隔完全一致
		grids[i] = solidGrid(200)
	}
	if !hasStasisRun(grids) {
		t.Fatal("存在约 10 秒静止段应命中")
	}

	// 静止段只有 4 个间隔（约 8 秒）：不应命中
	grids = make([][]uint8, 20)
	for i := range grids {
		grids[i] = solidGrid(uint8(30 + i*10))
	}
	for i := 6; i < 10; i++ { // 只有 4 个连续一致间隔
		grids[i] = solidGrid(200)
	}
	if hasStasisRun(grids) {
		t.Fatal("8 秒静止不应命中（阈值 10 秒）")
	}

	// 帧数不足：不应命中
	if hasStasisRun([][]uint8{solidGrid(1), solidGrid(1), solidGrid(1)}) {
		t.Fatal("帧数不足时不应命中")
	}
}

// TestMeanAbsDiff 钉住帧差计算。
func TestMeanAbsDiff(t *testing.T) {
	if got := meanAbsDiff(solidGrid(100), solidGrid(100)); got != 0 {
		t.Fatalf("相同网格差应为 0，实际 %.2f", got)
	}
	if got := meanAbsDiff(solidGrid(100), solidGrid(150)); got != 50 {
		t.Fatalf("差 50 的网格平均绝对差应为 50，实际 %.2f", got)
	}
	// 长度不一致按完全不同处理
	if got := meanAbsDiff(solidGrid(100), []uint8{1}); got != 255 {
		t.Fatalf("长度不一致应返回 255，实际 %.2f", got)
	}
}

// TestGrayGrid 钉住图像 → 16x16 灰度网格的降采样。
func TestGrayGrid(t *testing.T) {
	// 32x32 的纯白图
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	grid := grayGrid(img)
	if len(grid) != 256 {
		t.Fatalf("网格应为 256 值，实际 %d", len(grid))
	}
	for i, v := range grid {
		if v != 255 {
			t.Fatalf("纯白图降采样后应为 255，第 %d 值为 %d", i, v)
		}
	}
}

// TestAuditRejectLimitConstant 钉住连续拒绝上限，防止选片死循环。
func TestAuditRejectLimitConstant(t *testing.T) {
	if auditRejectMaxLimit < 1 || auditRejectMaxLimit > 5 {
		t.Fatalf("连续拒绝上限应在 1-5 之间，实际 %d", auditRejectMaxLimit)
	}
}
