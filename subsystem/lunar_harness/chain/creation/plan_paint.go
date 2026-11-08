package creation

// plan_paint.go 自主绘图目标池：题材 / 风格 / 画幅 / 批次，可通过配置文件扩展。

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// paintPools 绘图目标池。
type paintPools struct {
	// Scenes 画面题材池
	Scenes []string `json:"scenes"`
	// Styles 风格池
	Styles []string `json:"styles"`
	// Ratios 画幅池
	Ratios []string `json:"ratios"`
	// Batches 单轮出图张数池
	Batches []int `json:"batches"`
}

// defaultPaintPools 内置绘图目标池；配置文件存在时按字段覆盖。
var defaultPaintPools = paintPools{
	Scenes: []string{
		"雨夜霓虹街道上撑伞的旅人",
		"漂浮在云海之上的古老图书馆",
		"深夜便利店门口打哈欠的猫",
		"沙漠里缓慢行走的机械驼队",
		"被森林覆盖的废弃天文台",
		"春日溪边晾晒和服的少女",
		"深海热泉旁发光的珊瑚城市",
		"雪原上追逐极光的列车",
		"旧城区天台上的露天电影院",
		"月球基地窗外升起的地球",
		"黄昏码头边停靠的纸船",
		"书房里读书的猫头鹰学者",
	},
	Styles: []string{
		"电影感写实，柔和体积光",
		"水彩插画，留白与淡彩",
		"像素艺术，16 位色板",
		"浮世绘风格，平涂与描边",
		"赛博朋克霓虹配色",
		"童话绘本，温暖低饱和",
		"黑白线稿加单色点缀",
		"厚涂油画质感，强烈笔触",
	},
	Ratios:  []string{"1:1", "16:9", "3:4", "4:3"},
	Batches: []int{1, 1, 1, 2},
}

// loadPaintPools 读取绘图目标池：配置文件中的非空字段覆盖内置池。
func loadPaintPools() paintPools {
	pools := paintPools{
		Scenes:  append([]string(nil), defaultPaintPools.Scenes...),
		Styles:  append([]string(nil), defaultPaintPools.Styles...),
		Ratios:  append([]string(nil), defaultPaintPools.Ratios...),
		Batches: append([]int(nil), defaultPaintPools.Batches...),
	}
	raw, ok := persistence.ReadFileBytes(paintTargetsFile)
	if !ok {
		return pools
	}
	var override paintPools
	if err := json.Unmarshal(raw, &override); err != nil {
		logger.Warn(Module, "[自主创作] 绘图目标池配置损坏，使用内置池: %v", err)
		return pools
	}
	if len(override.Scenes) > 0 {
		pools.Scenes = override.Scenes
	}
	if len(override.Styles) > 0 {
		pools.Styles = override.Styles
	}
	if len(override.Ratios) > 0 {
		pools.Ratios = override.Ratios
	}
	if len(override.Batches) > 0 {
		pools.Batches = override.Batches
	}
	return pools
}

// buildPaint 从绘图目标池组合一次绘画创作。
func buildPaint(r *rand.Rand, _ qqTargets) (PlanItem, bool) {
	pools := loadPaintPools()
	scene := pickString(r, pools.Scenes)
	style := pickString(r, pools.Styles)
	ratio := pickString(r, pools.Ratios)
	batch := pickInt(r, pools.Batches)
	if batch < 1 {
		batch = 1
	}
	files := make([]string, 0, batch)
	for i := 0; i < batch; i++ {
		files = append(files, fmt.Sprintf("画面 %d", i+1))
	}
	task := fmt.Sprintf(
		"本轮做一次绘图创作：以「%s」为主题创作 %d 张作品，画幅 %s，风格要求「%s」。\n"+
			"必须安排 agent=painter 的步骤，并在 paint 中写明 prompt（画面描述）、ratio（画幅）、style（风格）；"+
			"多张时拆成多个 painter 步骤，每张都要有独立而具体的画面描述。\n"+
			"不要用 SVG、Canvas、HTML 或代码绘制代替图片；每张图完成后在 /document/创作/绘画/ 写入 manifest.md，"+
			"记录每张图的描述、风格、画幅与资源句柄。",
		scene, batch, ratio, style,
	)
	return PlanItem{
		Kind:  KindPaint,
		Title: fmt.Sprintf("画 %d 张「%s」", batch, scene),
		Params: map[string]string{
			"题材": scene,
			"风格": style,
			"画幅": ratio,
			"批次": strconv.Itoa(batch),
			"画面": strings.Join(files, "、"),
		},
		Task:     task,
		Activity: "刚画完一组" + scene,
	}, true
}

// paintSceneOf 返回事项对应的具体绘画题材，用于短时间去重。
func paintSceneOf(item PlanItem) string {
	if item.Kind != KindPaint {
		return ""
	}
	return strings.TrimSpace(item.Params["题材"])
}

// pickString 从池中随机取一个非空字符串；池为空时返回空串。
func pickString(r *rand.Rand, pool []string) string {
	values := make([]string, 0, len(pool))
	for _, item := range pool {
		if strings.TrimSpace(item) != "" {
			values = append(values, strings.TrimSpace(item))
		}
	}
	if len(values) == 0 {
		return ""
	}
	return values[r.Intn(len(values))]
}

// pickInt 从池中随机取一个正整数；池为空时返回 1。
func pickInt(r *rand.Rand, pool []int) int {
	values := make([]int, 0, len(pool))
	for _, item := range pool {
		if item > 0 {
			values = append(values, item)
		}
	}
	if len(values) == 0 {
		return 1
	}
	return values[r.Intn(len(values))]
}
