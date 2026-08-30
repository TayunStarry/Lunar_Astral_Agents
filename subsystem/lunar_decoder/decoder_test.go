package lunardecoder

import (
	"bytes"
	"errors"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// TestKeyGroupsZero 验证 key=0 派生出的 8 组均为全零操作（恒等组）。
func TestKeyGroupsZero(t *testing.T) {
	groups, err := KeyGroups(0)
	if err != nil {
		t.Fatalf("KeyGroups(0) error: %v", err)
	}
	for i, g := range groups {
		if g.Op != 0 || g.Count != 0 {
			t.Fatalf("group[%d] = %+v, want {0 0}", i, g)
		}
	}
}

// TestKeyGroupsValues 验证普通密钥的标准化与分组结果。
func TestKeyGroupsValues(t *testing.T) {
	// key=5 -> "5" -> 右补0 -> "5000000000000000" -> 首组 {5,0}，其余 {0,0}。
	groups, err := KeyGroups(5)
	if err != nil {
		t.Fatal(err)
	}
	want := [8]Group{
		{Op: 5, Count: 0}, {0, 0}, {0, 0}, {0, 0},
		{0, 0}, {0, 0}, {0, 0}, {0, 0},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Fatalf("KeyGroups(5)=%+v, want %+v", groups, want)
	}

	// key=-1 取 16 位无符号幅度 65535 -> "65535" -> "65535000000000000"。
	groups, err = KeyGroups(-1)
	if err != nil {
		t.Fatal(err)
	}
	if groups[0] != (Group{Op: 6, Count: 5}) || groups[1] != (Group{Op: 5, Count: 3}) {
		t.Fatalf("KeyGroups(-1) prefix unexpected: %+v", groups[:2])
	}
}

// TestCombineKeySegments 验证超长字符串的拆分、右补零与不进位加法路径。
func TestCombineKeySegments(t *testing.T) {
	// 1) 不足 16 位：右补 0。
	groups, err := combineKeySegments("123")
	if err != nil {
		t.Fatal(err)
	}
	if groups[0] != (Group{Op: 1, Count: 2}) {
		t.Fatalf("combine('123') group0=%+v, want {1 2}", groups[0])
	}

	// 2) 恰为 16 位：原样拆分。
	// "1234567890123456" -> 组0{1,2} 组1{3,4} 组2{5,6} 组3{7,8} 组4{9,0} 组5{1,2} 组6{3,4} 组7{5,6}。
	full16 := "1234567890123456"
	groups, err = combineKeySegments(full16)
	if err != nil {
		t.Fatal(err)
	}
	want16 := [8]Group{
		{1, 2}, {3, 4}, {5, 6}, {7, 8},
		{9, 0}, {1, 2}, {3, 4}, {5, 6},
	}
	if !reflect.DeepEqual(groups, want16) {
		t.Fatalf("combined(full16)=%+v, want %+v", groups, want16)
	}

	// 3) 超过 16 位：拆分两段并逐位不进位加法。
	// "12345678901234567890" -> 段1 "1234567890123456"，段2 "7890" 右补0 为 "7890000000000000"
	// 逐位取模求和 -> "8024567890123456"。
	groups, err = combineKeySegments("12345678901234567890")
	if err != nil {
		t.Fatal(err)
	}
	want := [8]Group{
		{8, 0}, {2, 4}, {5, 6}, {7, 8},
		{9, 0}, {1, 2}, {3, 4}, {5, 6},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Fatalf("combine(20digits)=%+v, want %+v", groups, want)
	}

	// 4) 空字符串回退为全零。
	groups, err = combineKeySegments("")
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range groups {
		if g != (Group{}) {
			t.Fatalf("empty combine unexpected: %+v", groups)
		}
	}
}

// TestRotateDigits 验证数字部分循环位移的精确语义。
func TestRotateDigits(t *testing.T) {
	in := []byte("0123456789")

	// 向后位移 1 位：索引 +1。
	out := transformGroup(in, Group{Op: 0, Count: 1}, false)
	if string(out) != "1234567890" {
		t.Fatalf("backward rotate got %q, want %q", out, "1234567890")
	}

	// 向前位移 1 位：索引 -1。
	out = transformGroup(in, Group{Op: 1, Count: 1}, false)
	if string(out) != "9012345678" {
		t.Fatalf("forward rotate got %q, want %q", out, "9012345678")
	}
}

// TestRotateLetters 验证字母部分循环位移的精确语义（含大小写分界）。
func TestRotateLetters(t *testing.T) {
	in := []byte("abc")

	// 向后位移 1 位：a->b, b->c, c->d。
	out := transformGroup(in, Group{Op: 2, Count: 1}, false)
	if string(out) != "bcd" {
		t.Fatalf("letters backward got %q, want %q", out, "bcd")
	}

	// 向前位移 1 位：b->a, c->b, d->c。
	out = transformGroup([]byte("bcd"), Group{Op: 3, Count: 1}, false)
	if string(out) != "abc" {
		t.Fatalf("letters forward got %q, want %q", out, "abc")
	}
}

// TestRotateSymbols 验证符号部分循环位移的精确语义。
func TestRotateSymbols(t *testing.T) {
	in := []byte("+/=")

	// 向后：+->/, /->=, =->+。
	out := transformGroup(in, Group{Op: 4, Count: 1}, false)
	if string(out) != "/=+" {
		t.Fatalf("symbols backward got %q, want %q", out, "/=+")
	}

	// 向前：+->=, /->+, =->/。
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
	// 单独覆盖类型 8/9。
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

// TestRoundTripKeys 使用多组密钥与多种数据验证编码/解码往返一致。
func TestRoundTripKeys(t *testing.T) {
	keys := []int16{0, 1, 7, 12345, 32767, -5, -32768, 100}
	data := [][]byte{
		[]byte{},
		[]byte("a"),
		[]byte("星月智能"),
		[]byte("The quick brown fox jumps over the lazy dog"),
		[]byte{0x00, 0x01, 0x02, 0x03, 0xff},
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
	// 文件名原样保留。
	for i, f := range out {
		if f.Name != in[i].Name {
			t.Fatalf("name changed at %d: %q -> %q", i, in[i].Name, f.Name)
		}
	}
	dec, err := DecodeFiles(out, 1234)
	if err != nil {
		t.Fatalf("DecodeFiles err=%v", err)
	}
	if !reflect.DeepEqual(dec[0].Data, in[0].Data) {
		t.Fatal("multi-file round trip mismatch")
	}
	if !bytes.Equal(dec[0].Data, in[0].Data) {
		t.Fatalf("a mismatch: %q", dec[0].Data)
	}
	if dec[1].Name != "b.bin" {
		t.Fatalf("b name mismatch: %q", dec[1].Name)
	}
}

// TestDecodeInvalidBase64 验证还原出的非法 Base64 触发错误。
func TestDecodeInvalidBase64(t *testing.T) {
	// key=0 派生全零恒等组，Data 直接作为 Base64 解码必失败。
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

// TestRoundTripKeyString 验证数字字符串密钥（含 34 位超长密钥）的编解码往返。
func TestRoundTripKeyString(t *testing.T) {
	keyStrs := []string{
		"1234567890123456",                   // 恰好 16 位
		"0",                                  // 不足 16 位
		"135791113151719212325272931",        // 34 位（拆两段 16+16+2 补零合并）
		"0000000000000000000000000000000000", // 全零 34 位
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

// TestKeyStringInvalid 验证非法或空密钥字符串的错误处理。
func TestKeyStringInvalid(t *testing.T) {
	if _, err := EncodeFilesWithKeyString([]FileData{{Data: []byte("x")}}, ""); err == nil {
		t.Fatal("empty key should error")
	}
	if _, err := EncodeFilesWithKeyString([]FileData{{Data: []byte("x")}}, "12ab45"); err == nil {
		t.Fatal("non-numeric key should error")
	}
	if _, err := DecodeFilesWithKeyString([]FileData{{Data: []byte("x")}}, "-12"); err == nil {
		t.Fatal("negative key string should error")
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

// TestEncodeOutputWithinBase64Charset 验证编码产物只含 Base64 字符集（64 字符 + 填充 '='）内的
// 可打印文本，不会出现任意字节，确保 .enc 可安全存入 JSON / 文本传输场景。
// 覆盖能派生操作类型 8/9 的密钥（int16 与超长字符串密钥）。
func TestEncodeOutputWithinBase64Charset(t *testing.T) {
	const charset = "+/0123456789=ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	keys := []int16{0, 5, 12345, 32767, -5, -32768} // -32768 -> uint16 32768 -> 组 {8,0}（操作 8）
	keyStrs := []string{
		"8800000000000000", // 组 {8,8}（操作 8）
		"9900000000000000", // 组 {9,9}（操作 9）
	}
	data := [][]byte{
		[]byte("hello world"),
		[]byte{0x00, 0x01, 0x02, 0xff, 0xfe}, // 二进制数据必然产生 Base64 填充 '='
		makeRandomBytes(t, 300),
	}

	checkCharset := func(name string, out []byte) {
		for _, b := range out {
			if !strings.ContainsRune(charset, rune(b)) {
				t.Fatalf("%s: 输出字节 0x%02x 在 Base64 字符集之外: %q", name, b, out)
			}
		}
	}

	for _, key := range keys {
		for _, d := range data {
			enc, err := EncodeFiles([]FileData{{Name: "f", Data: d}}, key)
			if err != nil {
				t.Fatalf("EncodeFiles key=%d err=%v", key, err)
			}
			checkCharset("int16 key="+strconv.Itoa(int(key)), enc[0].Data)
		}
	}
	for _, ks := range keyStrs {
		for _, d := range data {
			enc, err := EncodeFilesWithKeyString([]FileData{{Name: "f", Data: d}}, ks)
			if err != nil {
				t.Fatalf("EncodeFilesWithKeyString %q err=%v", ks, err)
			}
			checkCharset("string key="+ks, enc[0].Data)
		}
	}
}

// TestXorChainBarrier 验证操作 8/9 在字符集外字符（'='）作为边界时，链式索引异或仍严格可逆。
func TestXorChainBarrier(t *testing.T) {
	groups89 := [8]Group{
		{Op: 8, Count: 1}, {Op: 9, Count: 2},
		{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	}
	samples := [][]byte{
		// 含 '=' 边界且出现在中间位置（操作 4/5 或 6/7 可能把 '+' 位移为 '='）。
		[]byte("ab=c"),
		[]byte("a=bc"),
		[]byte("=abc"),
		[]byte("ab==c"),
		[]byte("=="),
		[]byte("abcdefghijklmnopqrstuvwxyz+/=0123456789"),
	}
	for _, s := range samples {
		enc := ApplyTransform(s, groups89, false)
		dec := ApplyTransform(enc, groups89, true)
		if !bytes.Equal(dec, s) {
			t.Fatalf("xor chain barrier round trip failed: in %q -> %q -> %q", s, enc, dec)
		}
	}
}

// TestRLE 验证密钥绑定的游程编码解码还原，覆盖转义、控制字节冲突、超长游程与非法格式。
func TestRLE(t *testing.T) {
	samples := [][]byte{
		nil,
		[]byte{},
		[]byte("hello world"),
		[]byte{0, 0, 0, 0},                   // 全零，与控制字节可能冲突
		bytes.Repeat([]byte{'a'}, 300),       // 超长游程（按 255 分段）
		[]byte{7, 7, 7, 7, 7, 7, 7, 7},       // 控制字节 7 的长游程
		{0, 1, 2, 3, 4, 5, 6, 7, 7, 7, 7, 7}, // 含控制字节 7 与转义
		makeRandomBytes(t, 500),
	}
	for _, control := range []byte{0, 7, 255} {
		for _, s := range samples {
			enc := rleEncode(s, control)
			dec, err := rleDecode(enc, control)
			if err != nil {
				t.Fatalf("rle control=%d err=%v", control, err)
			}
			if !bytes.Equal(dec, s) {
				t.Fatalf("rle round trip control=%d failed: in %v -> %v -> %v", control, s, enc, dec)
			}
		}
	}
	// 非法格式（截断 / 越界）报错。
	if _, err := rleDecode([]byte{7}, 7); !errors.Is(err, ErrInvalidRLE) {
		t.Fatalf("want ErrInvalidRLE, got %v", err)
	}
	if _, err := rleDecode([]byte{7, 3}, 7); !errors.Is(err, ErrInvalidRLE) {
		t.Fatalf("want ErrInvalidRLE, got %v", err)
	}
}

func makeRandomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	r := rand.New(rand.NewSource(42))
	_, _ = r.Read(b)
	return b
}
