package module

// EncodePCMToWAV 将 float32 PCM 采样编码为 16bit 单声道 WAV
func EncodePCMToWAV(samples []float32, sampleRate int) []byte {
	numSamples := len(samples)
	byteRate := sampleRate * 2
	blockAlign := 2
	dataSize := numSamples * 2
	fileSize := 36 + dataSize

	buf := make([]byte, 44+dataSize)

	buf[0] = 'R'
	buf[1] = 'I'
	buf[2] = 'F'
	buf[3] = 'F'
	putUint32LE(buf[4:], uint32(fileSize))
	buf[8] = 'W'
	buf[9] = 'A'
	buf[10] = 'V'
	buf[11] = 'E'
	buf[12] = 'f'
	buf[13] = 'm'
	buf[14] = 't'
	buf[15] = ' '
	putUint32LE(buf[16:], 16)
	putUint16LE(buf[20:], 1)
	putUint16LE(buf[22:], 1)
	putUint32LE(buf[24:], uint32(sampleRate))
	putUint32LE(buf[28:], uint32(byteRate))
	putUint16LE(buf[32:], uint16(blockAlign))
	putUint16LE(buf[34:], 16)
	buf[36] = 'd'
	buf[37] = 'a'
	buf[38] = 't'
	buf[39] = 'a'
	putUint32LE(buf[40:], uint32(dataSize))

	for i, sample := range samples {
		val := int16(max(-32768, min(32767, int32(sample*32767.0))))
		offset := 44 + i*2
		buf[offset] = byte(val)
		buf[offset+1] = byte(val >> 8)
	}

	return buf
}

func putUint16LE(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func putUint32LE(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}
