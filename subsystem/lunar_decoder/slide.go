package lunardecoder

// SlideDirection 表示上下文窗口滑移的方向。
type SlideDirection int

const (
	// SlideNone 不滑移。
	SlideNone SlideDirection = iota
	// SlideBackward 向后滑移。
	SlideBackward
	// SlideForward 向前滑移。
	SlideForward
)

// SlideSpec 描述滑移方向及奇偶统计。
type SlideSpec struct {
	// Direction 滑移方向。
	Direction SlideDirection
	// EvenCount 偶数组数量。
	EvenCount int
	// OddCount 奇数组数量。
	OddCount int
}

// groupValue 将小组换算为数值，用于奇偶判断。
func groupValue(g Group) int {
	return g.Op*10 + g.Count
}

// SlideSpecFromGroups 按 8 组奇偶统计确定滑移方向；奇多于偶向前，否则向后。
func SlideSpecFromGroups(groups [8]Group) SlideSpec {
	spec := SlideSpec{}
	for _, g := range groups {
		if groupValue(g)%2 == 0 {
			spec.EvenCount++
		} else {
			spec.OddCount++
		}
	}
	if spec.OddCount > spec.EvenCount {
		spec.Direction = SlideForward
	} else {
		spec.Direction = SlideBackward
	}
	return spec
}

// SlideAmount 计算滑移次数：密钥数值对密文长度取模。
func SlideAmount(keyValue string, length int) int {
	if length <= 0 {
		return 0
	}
	base := keyIndexBase
	r := 0
	for i := 0; i < len(keyValue); i++ {
		v := charValue(keyValue[i])
		if v < 0 {
			v = 0
		}
		r = (r*base + v) % length
	}
	return r
}

// SlideRotate 将字节串环形滚动 amount 次（双射；向后 k 次逆操作即向前 k 次）。
func SlideRotate(s []byte, direction SlideDirection, amount int) []byte {
	out := append([]byte(nil), s...)
	n := len(s)
	if n == 0 || amount == 0 || direction == SlideNone {
		return out
	}
	k := amount % n
	if k == 0 {
		return out
	}
	switch direction {
	case SlideBackward:
		for i := 0; i < n; i++ {
			out[i] = s[(i-k+n)%n]
		}
	case SlideForward:
		for i := 0; i < n; i++ {
			out[i] = s[(i+k)%n]
		}
	}
	return out
}

// ReverseDirection 返回相反滑移方向。
func ReverseDirection(direction SlideDirection) SlideDirection {
	switch direction {
	case SlideBackward:
		return SlideForward
	case SlideForward:
		return SlideBackward
	default:
		return SlideNone
	}
}
