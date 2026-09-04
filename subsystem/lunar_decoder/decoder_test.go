package lunardecoder

import (
	"bytes"
	"errors"
	"math/rand"
	"reflect"
	"testing"
)

// ---------- 密钥处理 ----------

// TestKeyGroupsZero 验证 key=0（单字符密钥）补圆周率后派生合法单轮，而非全零恒等组。
func TestKeyGroupsZero(t *testing.T) {
	groups, err := KeyGroups(0)
	if err != nil {
		t.Fatalf("KeyGroups(0) error: %v", err)
	}
	// key=0："0" + 圆周率前 15 位 -> 首组 {0,3}（控制位0、次数位'1'编号3）。
	if groups[0] != (Group{Op: 0, Count: 3}) {
		t.Fatalf("group[0] = %+v, want {0 3}", groups[0])
	}
}

// TestKeyGroupsSingleDigitPadPi 验证 1 字符密钥用圆周率第 1 位起补齐，并核对派生小组。
func TestKeyGroupsSingleDigitPadPi(t *testing.T) {
	rounds, err := KeyRoundsFromString("5")
	if err != nil {
		t.Fatal(err)
	}
	if len(rounds) != 1 {
		t.Fatalf("len(rounds)=%d, want 1", len(rounds))
	}
	// 1 个字符 -> 从圆周率小数点后第 (1-1)*20+1=1 位起补 15 位 "141592653589793"。
	wantKey := "5" + piDecimals[:15]
	if rounds[0].Key != wantKey {
		t.Fatalf("round key=%q, want %q", rounds[0].Key, wantKey)
	}
	// 首组：控制位 '5'（数字值5，Op=5，extra=0），次数位 '1'（编码号3）-> {5,3}。
	if rounds[0].Groups[0] != (Group{Op: 5, Count: 3}) {
		t.Fatalf("groups[0]=%+v, want {5 3}", rounds[0].Groups[0])
	}
}

// TestKeyGroupsExact16 验证恰 16 位密钥不补齐。
func TestKeyGroupsExact16(t *testing.T) {
	key := "1234567890123456"
	rounds, err := KeyRoundsFromString(key)
	if err != nil {
		t.Fatal(err)
	}
	if len(rounds) != 1 || rounds[0].Key != key {
		t.Fatalf("exact16 rounds=%+v", rounds)
	}
	// ('1','2'): 控制位1，次数位'2'索引4 -> {1,4}。
	if rounds[0].Groups[0] != (Group{Op: 1, Count: 4}) {
		t.Fatalf("groups[0]=%+v, want {1 4}", rounds[0].Groups[0])
	}
	want := [8]Group{
		{1, 4}, {3, 6}, {5, 8}, {7, 10},
		{9, 2}, {1, 4}, {3, 6}, {5, 8},
	}
	if !reflect.DeepEqual(rounds[0].Groups, want) {
		t.Fatalf("groups=%+v, want %+v", rounds[0].Groups, want)
	}
}

// TestKeyGroupsOver16SplitPadd 验证超过 16 位密钥拆分为多轮，末轮用圆周率补齐。
func TestKeyGroupsOver16SplitPadd(t *testing.T) {
	key := "1234567890123456ABC" // 19 字符 -> 16 + 3
	rounds, err := KeyRoundsFromString(key)
	if err != nil {
		t.Fatal(err)
	}
	if len(rounds) != 2 {
		t.Fatalf("len(rounds)=%d, want 2", len(rounds))
	}
	if rounds[0].Key != "1234567890123456" {
		t.Fatalf("round0 key=%q", rounds[0].Key)
	}
	// 第 2 轮 3 个字符 -> 从圆周率第 (3-1)*20+1=41 位起（0 索引 40）补 13 位。
	wantRound1 := "ABC" + piDecimals[40:53]
	if rounds[1].Key != wantRound1 {
		t.Fatalf("round1 key=%q, want %q", rounds[1].Key, wantRound1)
	}
	// ('A','B'): 控制位 'A' 编号13 -> Op=3, extra=1；次数位 'B' 编号14 -> Count=14+1=15。
	if rounds[1].Groups[0] != (Group{Op: 3, Count: 15}) {
		t.Fatalf("round1 groups[0]=%+v, want {3 15}", rounds[1].Groups[0])
	}
}

// TestGroupDerivationSymbolOp 验证控制位为符号时按字符集编号映射（%10 与 /10 叠加次数）。
func TestGroupDerivationSymbolOp(t *testing.T) {
	// "A": 控制位 'A' 编号13 -> Op=13%10=3, extra=13/10=1。
	rounds, err := KeyRoundsFromString("A")
	if err != nil {
		t.Fatal(err)
	}
	want := [8]Group{{3, 4}, {4, 3}, {5, 11}, {2, 8}, {5, 5}, {5, 10}, {9, 9}, {9, 5}}
	if !reflect.DeepEqual(rounds[0].Groups, want) {
		t.Fatalf("A groups=%+v, want %+v", rounds[0].Groups, want)
	}

	// "z": 控制位 'z' 编号64 -> Op=64%10=4, extra=6；次数位 '1' 索引3 -> Count=3+6=9。
	rounds, err = KeyRoundsFromString("z")
	if err != nil {
		t.Fatal(err)
	}
	if rounds[0].Groups[0] != (Group{Op: 4, Count: 9}) {
		t.Fatalf("z groups[0]=%+v, want {4 9}", rounds[0].Groups[0])
	}
}

// TestKeyStringInvalid 验证非法或空密钥字符串的错误处理。
func TestKeyStringInvalid(t *testing.T) {
	if _, err := EncodeFilesWithKeyString([]FileData{{Data: []byte("x")}}, ""); err == nil {
		t.Fatal("empty key should error")
	}
	// '!' 不在 65 字符密钥集中 -> 非法（此前 'a'/'b' 已属于扩展后的字符集）。
	if _, err := EncodeFilesWithKeyString([]FileData{{Data: []byte("x")}}, "12!45"); err == nil {
		t.Fatal("out-of-set key should error")
	}
	if _, err := EncodeFilesWithKeyString([]FileData{{Data: []byte("x")}}, "你好"); err == nil {
		t.Fatal("multibyte key should error")
	}
}

// ---------- 字符变换 ----------

// TestRotateDigits 验证数字部分循环位移的精确语义。
func TestRotateDigits(t *testing.T) {
	in := []byte("0123456789")

	out := transformGroup(in, Group{Op: 0, Count: 1}, false)
	if string(out) != "1234567890" {
		t.Fatalf("backward rotate got %q, want %q", out, "1234567890")
	}

	out = transformGroup(in, Group{Op: 1, Count: 1}, false)
	if string(out) != "9012345678" {
		t.Fatalf("forward rotate got %q, want %q", out, "9012345678")
	}
}

// TestRotateLetters 验证字母部分循环位移的精确语义（含大小写分界）。
func TestRotateLetters(t *testing.T) {
	in := []byte("abc")

	out := transformGroup(in, Group{Op: 2, Count: 1}, false)
	if string(out) != "bcd" {
		t.Fatalf("letters backward got %q, want %q", out, "bcd")
	}

	out = transformGroup([]byte("bcd"), Group{Op: 3, Count: 1}, false)
	if string(out) != "abc" {
		t.Fatalf("letters forward got %q, want %q", out, "abc")
	}
}

// TestRotateSymbols 验证符号部分循环位移的精确语义。
func TestRotateSymbols(t *testing.T) {
	in := []byte("+/=")

	out := transformGroup(in, Group{Op: 4, Count: 1}, false)
	if string(out) != "/=+" {
		t.Fatalf("symbols backward got %q, want %q", out, "/=+")
	}

	out = transformGroup(in, Group{Op: 5, Count: 1}, false)
	if string(out) != "=+/" {
		t.Fatalf("symbols forward got %q, want %q", out, "=+/")
	}
}

// TestTransformRoundTripAllTypes 覆盖操作类型 0-9 全部分支的编解码往返。
func TestTransformRoundTripAllTypes(t *testing.T) {
	groups := [8]Group{
		{Op: 0, Count: 3}, {Op: 1, Count: 2},
		{Op: 2, Count: 5}, {Op: 3, Count: 1},
		{Op: 4, Count: 4}, {Op: 5, Count: 6},
		{Op: 6, Count: 2}, {Op: 7, Count: 3},
	}
	groups89 := [8]Group{
		{Op: 8, Count: 5}, {Op: 9, Count: 7},
		{Op: 0, Count: 0}, {Op: 0, Count: 0},
		{Op: 0, Count: 0}, {Op: 0, Count: 0},
		{Op: 0, Count: 0}, {Op: 0, Count: 0},
	}

	samples := [][]byte{
		nil,
		[]byte("hello world"),
		[]byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz+/="),
		{0x00, 0x01, 0x02, 0xff, 0xfe},
	}

	for _, g := range [][8]Group{groups, groups89} {
		for _, s := range samples {
			enc := ApplyTransform(s, g, false)
			dec := ApplyTransform(enc, g, true)
			if !bytes.Equal(dec, s) {
				t.Fatalf("round trip failed for groups %+v, in %q -> %q -> %q", g, s, enc, dec)
			}
		}
	}
}

// ---------- 游程编码 ----------

// TestRLEBasic 验证 RLE 编码/解码往返与转义/游程规则。
func TestRLEBasic(t *testing.T) {
	control := byte(0xAA)
	cases := [][]byte{
		nil,
		[]byte("a"),
		[]byte("aaaa"),
		[]byte("aaaabbbbccccdd"),
		bytes.Repeat([]byte{0x7F}, 300),
		[]byte("hello world this is a test"),
	}
	for _, c := range cases {
		enc := rleEncode(c, control)
		dec, err := rleDecode(enc, control)
		if err != nil {
			t.Fatalf("rleDecode err=%v for %q", err, c)
		}
		if !bytes.Equal(dec, c) {
			t.Fatalf("RLE round trip mismatch: %q", c)
		}
	}
}

// TestRLEEscape 验证字面量 control 字节的转义（control,0）。
func TestRLEEscape(t *testing.T) {
	control := byte(0xAB)
	in := []byte{0x01, control, 0x02}
	enc := rleEncode(in, control)
	want := []byte{0x01, control, 0x00, 0x02}
	if !bytes.Equal(enc, want) {
		t.Fatalf("escape got %v, want %v", enc, want)
	}
	dec, err := rleDecode(enc, control)
	if err != nil || !bytes.Equal(dec, in) {
		t.Fatalf("escape decode got %v (err %v)", dec, err)
	}
}

// TestRLEInvalid 验证截断游程触发错误。
func TestRLEInvalid(t *testing.T) {
	control := byte(0xAB)
	if _, err := rleDecode([]byte{control}, control); !errors.Is(err, ErrInvalidRLE) {
		t.Fatalf("want ErrInvalidRLE, got %v", err)
	}
	if _, err := rleDecode([]byte{control, 0x03}, control); !errors.Is(err, ErrInvalidRLE) {
		t.Fatalf("want ErrInvalidRLE, got %v", err)
	}
}

// ---------- 上下文窗口滑移 ----------

// TestSlideSpecParity 验证奇偶统计与方向判定。
func TestSlideSpecParity(t *testing.T) {
	allEven := [8]Group{{0, 0}, {2, 0}, {4, 0}, {6, 0}, {8, 0}, {0, 2}, {2, 4}, {6, 8}}
	spec := SlideSpecFromGroups(allEven)
	if spec.Direction != SlideBackward || spec.EvenCount != 8 || spec.OddCount != 0 {
		t.Fatalf("all-even spec=%+v", spec)
	}

	allOdd := [8]Group{{1, 1}, {3, 3}, {5, 5}, {7, 7}, {9, 9}, {1, 3}, {5, 7}, {9, 1}}
	spec = SlideSpecFromGroups(allOdd)
	if spec.Direction != SlideForward || spec.EvenCount != 0 || spec.OddCount != 8 {
		t.Fatalf("all-odd spec=%+v", spec)
	}

	equal := [8]Group{{0, 0}, {2, 0}, {4, 0}, {6, 0}, {1, 1}, {3, 3}, {5, 5}, {7, 7}}
	spec = SlideSpecFromGroups(equal)
	if spec.Direction != SlideBackward || spec.EvenCount != 4 || spec.OddCount != 4 {
		t.Fatalf("equal spec=%+v", spec)
	}
}

// TestSlideAmountBase65 验证滑移次数按 65 字符集基数与 charValue 取值计算。
func TestSlideAmountBase65(t *testing.T) {
	// "10"：'1'->1、'0'->0，基数 65 -> (0*65+1)%3=1，(1*65+0)%3=2。
	if SlideAmount("10", 3) != 2 {
		t.Fatal("SlideAmount(\"10\",3) != 2")
	}
	// 数字与符号混合："A-" 等已在 65 集内。"z" 的编码号 64（>= base-1 位值合理）。
	if SlideAmount("z1", 5) < 0 || SlideAmount("z1", 5) >= 5 {
		t.Fatalf("z1 mod 5 out of range: %d", SlideAmount("z1", 5))
	}
	if SlideAmount("1234567890123456", 0) != 0 {
		t.Fatal("length 0 should give 0")
	}
	if got := SlideAmount("A141592653589793", 97); got < 0 || got >= 97 {
		t.Fatalf("mod97 out of range: %d", got)
	}
}

// TestSlideRotate 验证环形滚转的精确语义与往返。
func TestSlideRotate(t *testing.T) {
	if got := string(SlideRotate([]byte("abc"), SlideBackward, 1)); got != "cab" {
		t.Fatalf("backward got %q", got)
	}
	if got := string(SlideRotate([]byte("abc"), SlideForward, 1)); got != "bca" {
		t.Fatalf("forward got %q", got)
	}
	if got := string(SlideRotate([]byte("abc"), SlideBackward, 3)); got != "abc" {
		t.Fatalf("amount==len should be identity, got %q", got)
	}
	if got := string(SlideRotate([]byte("abc"), SlideNone, 5)); got != "abc" {
		t.Fatalf("SlideNone should be identity, got %q", got)
	}
	if got := SlideRotate([]byte{}, SlideBackward, 3); len(got) != 0 {
		t.Fatalf("empty should stay empty, got %q", got)
	}
	for _, k := range []int{1, 2, 5, 8} {
		s := []byte("lunar-decoder-window-slide")
		fwd := SlideRotate(s, SlideBackward, k)
		back := SlideRotate(fwd, SlideForward, k)
		if !bytes.Equal(back, s) {
			t.Fatalf("round trip k=%d failed", k)
		}
	}
	if ReverseDirection(SlideBackward) != SlideForward || ReverseDirection(SlideForward) != SlideBackward || ReverseDirection(SlideNone) != SlideNone {
		t.Fatal("ReverseDirection mapping wrong")
	}
}

// ---------- 规范扩散 ----------

// TestDiffuseRoundTrip 验证各长度数据经扩散/逆向扩散后完整还原。
func TestDiffuseRoundTrip(t *testing.T) {
	keys := []string{
		"1234567890123456",
		"A", // 不足16补圆周率
		"aBc+/=Z9q",
		"z",
		"+/0123456789=ABC",
	}
	data := [][]byte{
		{},
		{0x00},
		makeRandomBytes(t, 15),  // 不足一块
		makeRandomBytes(t, 16),  // 恰好一块
		makeRandomBytes(t, 40),  // 两块 + 尾部
		makeRandomBytes(t, 300), // 多块
		bytes.Repeat([]byte{0x7F}, 50),
	}
	for _, key := range keys {
		round := Round{Key: mustRoundKey(t, key), Groups: mustRoundGroups(t, key)}
		for _, d := range data {
			enc := diffuseRaw(d, round.Key, diffuseRounds, false)
			dec := diffuseRaw(enc, round.Key, diffuseRounds, true)
			if !bytes.Equal(dec, d) {
				t.Fatalf("diffuse round trip failed key=%q len=%d", key, len(d))
			}
		}
	}
}

// TestDiffuseAvalanche 验证单字节改动引发密文大面积变化（扩散/雪崩）。
func TestDiffuseAvalanche(t *testing.T) {
	key := "1234567890123456"
	data := makeRandomBytes(t, 128) // 8 个数据块
	enc := diffuseRaw(data, key, diffuseRounds, false)

	flipped := append([]byte(nil), data...)
	flipped[40] ^= 0x01 // 改动第 40 字节（第 3 块内部）
	enc2 := diffuseRaw(flipped, key, diffuseRounds, false)

	diff := 0
	for i := range enc {
		if enc[i] != enc2[i] {
			diff++
		}
	}
	// 因 CBC 链，改动点及其之后的所有块理应全变；128 字节里应有多数差异。
	if diff < len(enc)/2 {
		t.Fatalf("avalanche too weak: only %d/%d bytes differ", diff, len(enc))
	}
}

// TestDiffuseDeterministic 验证相同密钥与数据产出稳定密文。
func TestDiffuseDeterministic(t *testing.T) {
	key := "AbCdef+/=12345"
	data := []byte("deterministic diffusion")
	a := diffuseRaw(data, key, diffuseRounds, false)
	b := diffuseRaw(data, key, diffuseRounds, false)
	if !bytes.Equal(a, b) {
		t.Fatal("diffusion not deterministic")
	}
}

// mustRoundKey 返回 key 对应的对齐 16 字符轮密钥。
func mustRoundKey(t *testing.T, key string) string {
	t.Helper()
	rs, err := KeyRoundsFromString(key)
	if err != nil {
		t.Fatalf("KeyRoundsFromString(%q): %v", key, err)
	}
	return rs[0].Key
}

// mustRoundGroups 返回 key 首轮派生的 8 组操作。
func mustRoundGroups(t *testing.T, key string) [8]Group {
	t.Helper()
	g, err := KeyGroupsFromString(key)
	if err != nil {
		t.Fatalf("KeyGroupsFromString(%q): %v", key, err)
	}
	return g
}

// ---------- 编解码往返 ----------

// TestRoundTripKeys 使用多组整数密钥与多种数据验证编码/解码往返一致。
func TestRoundTripKeys(t *testing.T) {
	keys := []int16{0, 1, 7, 12345, 32767, -5, -32768, 100}
	data := [][]byte{
		[]byte{},
		[]byte("a"),
		[]byte("星月智能"),
		[]byte("The quick brown fox jumps over the lazy dog"),
		bytes.Repeat([]byte{0x00}, 100),
		makeRandomBytes(t, 300),
	}
	for _, key := range keys {
		for _, d := range data {
			enc, err := EncodeFiles([]FileData{{Name: "f", Data: d}}, key)
			if err != nil {
				t.Fatalf("EncodeFiles key=%d err=%v", key, err)
			}
			dec, err := DecodeFiles(enc, key)
			if err != nil {
				t.Fatalf("DecodeFiles key=%d err=%v", key, err)
			}
			if !bytes.Equal(dec[0].Data, d) {
				t.Fatalf("round trip mismatch key=%d", key)
			}
		}
	}
}

// TestRoundTripKeyString 验证字符串密钥（含短位圆周率补齐、符号、超长多轮）的往返。
func TestRoundTripKeyString(t *testing.T) {
	keyStrs := []string{
		"1234567890123456",            // 恰 16 位
		"0",                           // 不足 16 位 -> 圆周率补齐
		"A",                           // 符号控制位 + 圆周率补齐
		"aBc+/=Z9q",                   // 含符号，不足 16 位
		"135791113151719212325272931", // 30 位 -> 拆 16+14，末轮圆周率补齐
		"AbCdef1234567890+/=123",      // 21 位含符号 -> 16+5
		"++++++++++++++++",            // 16 位全符号
		"z",                           // 'z' 编码号 64，叠加较大执行次数
	}
	data := [][]byte{
		[]byte{},
		[]byte("hello 加密解密"),
		{0x00, 0x01, 0x02, 0xff, 0x10},
		makeRandomBytes(t, 512),
	}
	for _, ks := range keyStrs {
		for _, d := range data {
			enc, err := EncodeFilesWithKeyString([]FileData{{Name: "f", Data: d}}, ks)
			if err != nil {
				t.Fatalf("EncodeKS %q err=%v", ks, err)
			}
			dec, err := DecodeFilesWithKeyString(enc, ks)
			if err != nil {
				t.Fatalf("DecodeKS %q err=%v", ks, err)
			}
			if !bytes.Equal(dec[0].Data, d) {
				t.Fatalf("KS round trip mismatch key=%q", ks)
			}
		}
	}
}

// TestRoundTripMultipleFiles 验证多文件数组的逐一还原。
func TestRoundTripMultipleFiles(t *testing.T) {
	in := []FileData{
		{Name: "a.txt", Data: []byte("alpha")},
		{Name: "b.bin", Data: []byte{0x00, 0x01, 0x02}},
		{Name: "c.bin", Data: []byte{}},
	}
	out, err := EncodeFiles(in, 1234)
	if err != nil {
		t.Fatalf("EncodeFiles err=%v", err)
	}
	for i, f := range out {
		if f.Name != in[i].Name {
			t.Fatalf("name changed at %d: %q -> %q", i, in[i].Name, f.Name)
		}
	}
	dec, err := DecodeFiles(out, 1234)
	if err != nil {
		t.Fatalf("DecodeFiles err=%v", err)
	}
	for i := range in {
		if !bytes.Equal(dec[i].Data, in[i].Data) {
			t.Fatalf("multi-file round trip mismatch at %d", i)
		}
	}
}

// TestShortKeyPadUsesPiNotZero 验证短密钥用圆周率补齐而非补 0。
func TestShortKeyPadUsesPiNotZero(t *testing.T) {
	rounds, err := KeyRoundsFromString("0")
	if err != nil {
		t.Fatal(err)
	}
	// 1 个字符 -> 补圆周率前 15 位，首补位应为 '1' 而非 '0'。
	if len(rounds[0].Key) != 16 || rounds[0].Key[1] != '1' {
		t.Fatalf("short key padding should use pi, key=%q", rounds[0].Key)
	}
}

// TestDecodeInvalidBase64 验证还原出的非法 Base64 触发错误。
func TestDecodeInvalidBase64(t *testing.T) {
	_, err := DecodeFiles([]FileData{{Name: "x", Data: []byte("!!!not-base64!!!")}}, 0)
	if !errors.Is(err, ErrInvalidBase64) {
		t.Fatalf("want ErrInvalidBase64, got %v", err)
	}
}

// TestNilEncode 验证文件数组或数据为 nil 时的错误处理。
func TestNilEncode(t *testing.T) {
	if _, err := EncodeFiles(nil, 1); !errors.Is(err, ErrNilData) {
		t.Fatalf("EncodeFiles(nil) want ErrNilData, got %v", err)
	}
	if _, err := EncodeFiles([]FileData{{Name: "x", Data: nil}}, 1); !errors.Is(err, ErrNilData) {
		t.Fatalf("EncodeFiles(nil data) want ErrNilData, got %v", err)
	}
	if _, err := DecodeFiles(nil, 1); !errors.Is(err, ErrNilData) {
		t.Fatalf("DecodeFiles(nil) want ErrNilData, got %v", err)
	}
}

// TestEmptyInputRoundTrip 验证空文件数组往返。
func TestEmptyInputRoundTrip(t *testing.T) {
	out, err := EncodeFiles([]FileData{}, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("expected 0 files, got %d", len(out))
	}
}

func makeRandomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	r := rand.New(rand.NewSource(42))
	_, _ = r.Read(b)
	return b
}
