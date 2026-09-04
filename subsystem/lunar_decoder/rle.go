package lunardecoder

// rleSeed 由 8 组操作派生游程编码的控制字节（0-255）。
func rleSeed(groups [8]Group) byte {
	sum := 0
	for _, g := range groups {
		sum += g.Op*10 + g.Count
	}
	return byte(sum % 256)
}

// rleEncode 对数据做密钥绑定的游程编码：字面量 control 转义为 control,0，
// 长度 >= rleMinRun 的连续重复字节编码为 control,长度,字节，超长分段。
func rleEncode(data []byte, control byte) []byte {
	out := make([]byte, 0, len(data))
	n := len(data)
	for i := 0; i < n; {
		j := i + 1
		for j < n && data[j] == data[i] && j-i < 255 {
			j++
		}
		if run := j - i; run >= rleMinRun {
			out = append(out, control, byte(run), data[i])
		} else {
			for _, b := range data[i:j] {
				if b == control {
					out = append(out, control, 0)
				} else {
					out = append(out, b)
				}
			}
		}
		i = j
	}
	return out
}

// rleDecode 还原游程编码数据；格式非法时返回 [ErrInvalidRLE]。
func rleDecode(data []byte, control byte) ([]byte, error) {
	out := make([]byte, 0, len(data))
	n := len(data)
	for i := 0; i < n; {
		if data[i] == control {
			if i+1 >= n {
				return nil, ErrInvalidRLE
			}
			if data[i+1] == 0 {
				out = append(out, control)
				i += 2
				continue
			}
			if i+2 >= n {
				return nil, ErrInvalidRLE
			}
			run := int(data[i+1])
			for k := 0; k < run; k++ {
				out = append(out, data[i+2])
			}
			i += 3
			continue
		}
		out = append(out, data[i])
		i++
	}
	return out, nil
}
