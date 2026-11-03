package Painter

// frame.go 由画面比例 / 参照尺寸推导输出尺寸。

import (
	"math"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
)

func pixelBudget() float64 {
	budget := config.DiffusionPixelBudgetMP
	if budget <= 0 {
		budget = defaultBudget
	}
	return budget * 1_000_000
}

func alignSide(value float64) int {
	clamped := math.Min(math.Max(math.Round(value), 256), sideMax)
	return int(math.Min(math.Round(clamped/sideAlign)*sideAlign, sideMax))
}

func ratioPresets() map[string]DimensionPlan {
	result := map[string]DimensionPlan{}
	for key, item := range ratioBase {
		long := alignSide(sideBase * item.ratio)
		if item.horizontal {
			result[key] = DimensionPlan{Width: long, Height: sideBase, Origin: key + " 预设"}
		} else {
			result[key] = DimensionPlan{Width: sideBase, Height: long, Origin: key + " 预设"}
		}
	}
	return result
}

// planDimensions 由参照尺寸推导输出尺寸。
func planDimensions(reference *DimensionPlan) DimensionPlan {
	if reference == nil || reference.Width <= 0 || reference.Height <= 0 {
		return ratioPresets()[defaultRatio]
	}
	longRef := math.Max(float64(reference.Width), float64(reference.Height))
	shortRef := math.Min(float64(reference.Width), float64(reference.Height))
	ratio := longRef / shortRef
	desired := math.Min(sideBase, math.Min(math.Sqrt(pixelBudget()*budgetSlack/ratio), sideMax/ratio))
	lowerBound := math.Min(refSideMin, desired)
	upperBound := math.Max(lowerBound, desired)
	budgetLimit := pixelBudget() * budgetSlack * 1.02

	var best *struct {
		short      int
		long       int
		err        float64
		overBudget bool
	}
	for s := alignSide(lowerBound); float64(s) <= upperBound+sideAlign; s += sideAlign {
		if s > sideBase {
			break
		}
		l := alignSide(float64(s) * ratio)
		if l > sideMax {
			continue
		}
		overBudget := float64(s*l) > budgetLimit
		errValue := math.Abs(float64(l)/float64(s)-ratio) / ratio
		better := false
		if best == nil {
			better = true
		} else if best.overBudget != overBudget {
			better = !overBudget
		} else if !overBudget && math.Abs(errValue-best.err) <= math.Max(best.err, errValue)*0.01 && s*l > best.short*best.long {
			better = true
		} else if errValue < best.err-1e-9 {
			better = true
		}
		if better {
			best = &struct {
				short      int
				long       int
				err        float64
				overBudget bool
			}{short: s, long: l, err: errValue, overBudget: overBudget}
		}
	}
	if best == nil {
		l := alignSide(math.Min(sideMax, math.Max(refSideMin, sideBase)*ratio))
		best = &struct {
			short      int
			long       int
			err        float64
			overBudget bool
		}{short: alignSide(math.Max(refSideMin, float64(l)/ratio)), long: l, err: 1}
	}
	vertical := reference.Height >= reference.Width
	capped := best.err > 0.08
	origin := "参照图 " + itoa(reference.Width) + "x" + itoa(reference.Height) + " 比例"
	if capped {
		origin = "参照图 " + itoa(reference.Width) + "x" + itoa(reference.Height) + " 比例（受长边/像素上限限制，比例已尽力贴近）"
	}
	if vertical {
		return DimensionPlan{Width: best.short, Height: best.long, Origin: origin}
	}
	return DimensionPlan{Width: best.long, Height: best.short, Origin: origin}
}

// resolveDimensionPlan 解析画面比例预设。
func resolveDimensionPlan(ratio string, reference *DimensionPlan) DimensionPlan {
	key := strings.TrimSpace(ratio)
	key = strings.NewReplacer("×", ":", "x", ":", "X", ":", " ", "").Replace(key)
	if preset, ok := ratioPresets()[key]; ok {
		preset.Origin = key + " 预设"
		return preset
	}
	return planDimensions(reference)
}

func normalizeStrength(value float64, _ string) float64 {
	if value <= 0 {
		return 0.5
	}
	return math.Min(math.Max(value, 0.05), 0.95)
}

func normalizeSteps(value float64) int {
	if value <= 0 {
		return 0
	}
	return int(math.Round(math.Min(math.Max(value, 8), 60)))
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
