package lunardecoder

// 规范扩散层：对字节流做密钥约束的 Feistel-CBC 扩散，使单比特改动扩散到整段密文。

// diffuseRaw 对 data 做扩散，decode 为 false 加密、true 解密。
func diffuseRaw(data []byte, key string, rounds int, decode bool) []byte {
	if len(data) == 0 {
		return []byte{}
	}
	sk := feistelSubkeys(key, rounds)
	iv := feistelIV(key)

	n := len(data)
	out := make([]byte, n)
	prev := iv

	var b int
	for b = 0; b+diffuseBlock <= n; b += diffuseBlock {
		if decode {
			// 解密：Feistel 逆变换后再与前一块密文异或还原明文。
			var dec, cur [diffuseBlock]byte
			for i := 0; i < diffuseBlock; i++ {
				cur[i] = data[b+i]
			}
			dec = feistelDecrypt(cur, sk)
			for i := 0; i < diffuseBlock; i++ {
				out[b+i] = dec[i] ^ prev[i]
				prev[i] = cur[i]
			}
		} else {
			// 加密：明文与前一块密文异或后做 Feistel 正向变换。
			var blockIn, blockOut [diffuseBlock]byte
			for i := 0; i < diffuseBlock; i++ {
				blockIn[i] = data[b+i] ^ prev[i]
			}
			blockOut = feistelEncrypt(blockIn, sk)
			for i := 0; i < diffuseBlock; i++ {
				prev[i] = blockOut[i]
				out[b+i] = blockOut[i]
			}
		}
	}
	// 尾部：与前一块密文派生的字节流异或（加密/解密对称）。
	if tail := data[b:]; len(tail) > 0 {
		seed := byte(0)
		for i := 0; i < diffuseBlock; i++ {
			seed ^= prev[i]
		}
		for i := 0; i < len(tail); i++ {
			ks := prev[(i+1)%diffuseBlock] ^ seed ^ sk[i%len(sk)][0] ^ byte(i*17)
			out[b+i] = tail[i] ^ ks
		}
	}
	return out
}

// feistelSubkeys 由本轮密钥派生 rounds 组（每组 8 字节）子密钥。
func feistelSubkeys(key string, rounds int) [][]byte {
	out := make([][]byte, rounds)
	seed := byte(0)
	for i := 0; i < len(key); i++ {
		seed ^= byte(charValue(key[i]))
	}
	for r := 0; r < rounds; r++ {
		row := make([]byte, 8)
		for j := 0; j < 8; j++ {
			k := key[(r*2+j*3)%len(key)]
			row[j] = seed ^ byte(charValue(k)) ^ byte(r*31+j*17)
		}
		out[r] = row
	}
	return out
}

// feistelIV 由本轮密钥派生 CBC 初值（IV）。
func feistelIV(key string) [diffuseBlock]byte {
	var iv [diffuseBlock]byte
	seed := byte(0)
	for i := 0; i < len(key); i++ {
		seed ^= byte(charValue(key[i]))
	}
	for i := 0; i < diffuseBlock; i++ {
		iv[i] = seed ^ byte(charValue(key[i%len(key)])) ^ byte(i*19)
	}
	return iv
}

// feistelEncrypt 对单个数据块做 Feistel 正向加密。
func feistelEncrypt(blockIn [diffuseBlock]byte, sk [][]byte) [diffuseBlock]byte {
	block := blockIn
	for r := 0; r < len(sk); r++ {
		block = feistelRoundFwd(block, sk[r])
	}
	return block
}

// feistelDecrypt 对单个数据块做 Feistel 逆向解密。
func feistelDecrypt(blockIn [diffuseBlock]byte, sk [][]byte) [diffuseBlock]byte {
	block := blockIn
	for r := len(sk) - 1; r >= 0; r-- {
		block = feistelRoundInv(block, sk[r])
	}
	return block
}

// feistelRoundFwd 单个正向子轮：(L, R) -> (R, L^F(R))。
func feistelRoundFwd(block [diffuseBlock]byte, sk []byte) [diffuseBlock]byte {
	var l, r, f [8]byte
	for i := 0; i < 8; i++ {
		l[i] = block[i]
		r[i] = block[8+i]
	}
	f = feistelF(r, sk)
	var out [diffuseBlock]byte
	for i := 0; i < 8; i++ {
		out[i] = r[i]
		out[8+i] = l[i] ^ f[i]
	}
	return out
}

// feistelRoundInv 单个逆向子轮，是正向子轮的逆变换。
func feistelRoundInv(block [diffuseBlock]byte, sk []byte) [diffuseBlock]byte {
	var lp, rp, f [8]byte
	for i := 0; i < 8; i++ {
		lp[i] = block[i]
		rp[i] = block[8+i]
	}
	f = feistelF(lp, sk)
	var out [diffuseBlock]byte
	for i := 0; i < 8; i++ {
		out[i] = rp[i] ^ f[i]
		out[8+i] = lp[i]
	}
	return out
}

// feistelF 轮函数：非线性混合右半 r 与子密钥 sk，使输出强依赖右半全部字节。
func feistelF(r [8]byte, sk []byte) [8]byte {
	var f [8]byte
	acc := uint32(0)
	for i := 0; i < 8; i++ {
		acc = acc*2654435761 + uint32(r[i]) + uint32(sk[i]) + (acc >> 24)
	}
	for i := 0; i < 8; i++ {
		acc = acc*2654435761 + 1234567 + uint32(i)
		a := acc
		f[i] = byte(a) ^ byte(a>>8) ^ byte(a>>16) ^ byte(a>>24) ^ sk[(i*3)%8] ^ uint8(i)
	}
	return f
}
