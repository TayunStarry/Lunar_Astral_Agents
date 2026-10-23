package lunarloop

// archive_bridge_test.go 验证 Go↔JS 适配器边界上的入参解析
//
// 背景：端到端测试发现 resourceAdmit 收到了「空对象」，在资源目录里写出了
// kind="" / seq=0 的垃圾行。本测试直接建一个 goja 运行时、注册同一批适配器，
// 从 JS 侧按 TypeScript 的真实调用形态传参，断言参数被正确解析。

import (
	"LunarSubsystem/GeneralConfig"
	file "LunarSubsystem/LunarHarness/engine/persistence"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/dop251/goja"
)

// newBridgeTestRuntime 建一个注册了归档适配器的 goja 运行时（隔离的资源目录）
func newBridgeTestRuntime(t *testing.T) *goja.Runtime {
	t.Helper()
	tmp := t.TempDir()
	origLocal := *GeneralConfig.LocalDir
	origArchive := *GeneralConfig.ArchiveDBPath
	*GeneralConfig.LocalDir = tmp
	*GeneralConfig.ArchiveDBPath = filepath.Join(tmp, "database", "archive.db")
	t.Cleanup(func() {
		// 先关连接再还原配置，否则 Windows 上临时目录删不掉
		_ = file.CloseArchiveDB()
		*GeneralConfig.LocalDir = origLocal
		*GeneralConfig.ArchiveDBPath = origArchive
	})
	if err := os.MkdirAll(tmp, 0755); err != nil {
		t.Fatalf("准备临时目录失败: %v", err)
	}
	vm := goja.New()
	registerAdaptersToRuntime(vm)
	return vm
}

// TestResourceAdmitBridgeParsesObject 断言 JS 侧传入的普通对象字段被完整解析
func TestResourceAdmitBridgeParsesObject(t *testing.T) {
	vm := newBridgeTestRuntime(t)

	// 与 TypeScript 侧 admitResource 的真实调用形态一致（对象字面量 + base64 字节）
	script := `
		var out = resourceAdmit({
			sourceKind: "bytes",
			source: "",
			data: "aGVsbG8gd29ybGQ=",
			name: "note.md",
			kindHint: "D",
			origin: "client-upload",
			retain: "normal",
			mime: "text/markdown"
		});
		JSON.stringify({
			err: out[1] ? String(out[1]) : null,
			result: out[0]
		});
	`
	value, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("执行 JS 失败: %v", err)
	}
	payload := value.String()
	t.Logf("resourceAdmit 返回: %s", payload)
	if got := payload; len(got) == 0 {
		t.Fatalf("返回为空")
	}
	// 必须真的建出资源：句柄形如 @D.00x，且 name 与 kindHint 生效
	if !contains(payload, `"handle":"@D.`) {
		t.Errorf("未按 kindHint 生成文档句柄，实际: %s", payload)
	}
	if !contains(payload, `"name":"note.md"`) {
		t.Errorf("name 字段未被解析，实际: %s", payload)
	}
}

// TestResourceSetSectionsBridgeParsesArray 断言段落数组（对象数组）被完整解析
func TestResourceSetSectionsBridgeParsesArray(t *testing.T) {
	vm := newBridgeTestRuntime(t)

	script := `
		var admitted = resourceAdmit({
			sourceKind: "bytes",
			data: "IyDkuIDnr4kK5q2j5paHCg==",
			name: "doc.md",
			kindHint: "D",
			origin: "client-upload"
		 });
		var res = admitted[0];
		var ok = resourceSetSections(res.id, [
			{ ordinal: 1, level: 1, title: "一、标题", path: "一、标题", lineStart: 1, lineEnd: 2, chars: 8 }
		]);
		JSON.stringify({
			admitErr: admitted[1] ? String(admitted[1]) : null,
			id: res.id,
			setOk: ok[0],
			setErr: ok[1] ? String(ok[1]) : null,
			read: (function () {
				var r = resourceRead(res.handle + "|§1", 0);
				return r[1] ? ("ERR:" + String(r[1])) : (r[0] ? r[0].content : null);
			})()
		});
	`
	value, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("执行 JS 失败: %v", err)
	}
	payload := value.String()
	t.Logf("段落表往返: %s", payload)
	if !contains(payload, `"setOk":true`) {
		t.Errorf("resourceSetSections 未成功，实际: %s", payload)
	}
	if contains(payload, `"read":"ERR:`) || contains(payload, `"read":null`) {
		t.Errorf("按章节锚点取回失败，实际: %s", payload)
	}
}

// TestResourceAdmitBridgeRejectsEmptyInput 断言空入参被明确拒绝，而不是写出垃圾行
func TestResourceAdmitBridgeRejectsEmptyInput(t *testing.T) {
	vm := newBridgeTestRuntime(t)

	script := `
		var a = resourceAdmit({});
		var b = resourceAdmit({ sourceKind: "bytes", data: "" });
		var c = resourceAdmit({ sourceKind: "bytes", data: "aGk=", name: "x.txt" });
		JSON.stringify({
			emptyRejected: a[0] === null && !!a[1],
			emptyDataRejected: b[0] === null && !!b[1],
			validAccepted: c[0] !== null && c[1] === null
		});
	`
	value, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("执行 JS 失败: %v", err)
	}
	payload := value.String()
	t.Logf("空入参处理: %s", payload)
	if !contains(payload, `"emptyRejected":true`) {
		t.Errorf("空对象入参应被拒绝，实际: %s", payload)
	}
	if !contains(payload, `"emptyDataRejected":true`) {
		t.Errorf("空字节入参应被拒绝，实际: %s", payload)
	}
	if !contains(payload, `"validAccepted":true`) {
		t.Errorf("合法入参应被接受，实际: %s", payload)
	}
}

// TestImagePipelineResolvesResourceBlob 回归：图片处理管线必须能按资源目录的相对路径取到实体
//
// 现场缺陷：绘制者拿到 @I.001 后 call resizeImage("file://multimedia/<sha16>.jpg") 失败，
// 表现为「该图源已失效，未能成功选定基座」——而实体其实好好地躺在 multimedia/ 下。
// 根因是 file:// 去掉协议后是**相对路径**，而 Go 侧只按绝对路径读文件。
//
// resizeImage 现已从 goja 卸载（思维链不允许自己缩放图片，见 docs/adr/0008）：
// 同一个「按 blob_path 取到实体」的能力改由 visionRead 提供，本测试随之改测视觉管线。
func TestImagePipelineResolvesResourceBlob(t *testing.T) {
	vm := newBridgeTestRuntime(t)

	// 用标准库生成一张合法 PNG（手写字节极易造出 zlib 校验和不符的假图）
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := 0; x < 4; x++ {
		for y := 0; y < 4; y++ {
			img.Set(x, y, color.RGBA{R: 200, G: 120, B: 90, A: 255})
		}
	}
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("生成测试 PNG 失败: %v", err)
	}
	payloadB64 := base64.StdEncoding.EncodeToString(buf.Bytes())

	script := `
		var admitted = resourceAdmit({
			sourceKind: "bytes",
			data: "` + payloadB64 + `",
			name: "pic.png",
			kindHint: "I",
			origin: "client-upload"
		});
		var res = admitted[0];
		var out = { admitErr: admitted[1] ? String(admitted[1]) : null, handle: res ? res.handle : null, blob: res ? res.blob_path : null };
		if (res) {
			var a = visionRead(res.blob_path, res.handle);
			out.blobErr = a[1] ? String(a[1]) : null;
			out.blobOk = !a[1] && !!a[0] && a[0].indexOf("data:image/jpeg;base64,") === 0;
			out.blobW = a[2]; out.blobH = a[3];
			var c = visionRead("multimedia/definitely-missing.png", "@I.999");
			out.missingRejected = c[0] === null && !!c[1];
			out.missingErr = c[1] ? String(c[1]) : null;
		}
		out.resizeGone = (typeof resizeImage === "undefined");
		JSON.stringify(out);
	`
	value, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("执行 JS 失败: %v", err)
	}
	payload := value.String()
	t.Logf("图片管线解析结果: %s", payload)

	if !contains(payload, `"admitErr":null`) {
		t.Fatalf("资源入库失败: %s", payload)
	}
	if !contains(payload, `"blobOk":true`) {
		t.Errorf("按 blob_path 应能取到视觉副本: %s", payload)
	}
	if !contains(payload, `"blobW":4`) || !contains(payload, `"blobH":4`) {
		t.Errorf("视觉副本应回传宽高（4x4）: %s", payload)
	}
	if !contains(payload, `"missingRejected":true`) {
		t.Errorf("不存在的 blob 引用应明确报错: %s", payload)
	}
	if !contains(payload, `"resizeGone":true`) {
		t.Errorf("resizeImage 不应再暴露给智能体运行时: %s", payload)
	}
}

// TestResourceAdmitDataURIRoundTripsBytes 回归：data URI 入库后实体字节必须与原图**逐字节一致**
//
// 现场事故：图片入库曾被改成「JS 侧用 atob 解码 base64 → 传 bytes 给 Go」。而 atob 适配器
// 返回的是 Go 字符串，非法 UTF-8 字节在转成 JS 字符串时被逐个替换为 U+FFFD，
// 再经 charCodeAt 取回时低字节成了 0xFD —— 入库的图片于是全是 0xFD 垃圾（187KB，45% 是 0xFD），
// 表现为「图片尺寸探测失败」「ResizeImage 格式 unknown」「参考图已失效」。
//
// 本测试把真实 PNG 以 data URI 入库，再读回磁盘实体逐字节比对：
// 只要有人重新在 JS 侧引入二进制解码，这里立刻失败。
func TestResourceAdmitDataURIRoundTripsBytes(t *testing.T) {
	vm := newBridgeTestRuntime(t)

	// 真实 PNG（含 0x80 以上字节，足以触发 UTF-8 解释失真）
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 31), G: uint8(y * 31), B: 0xFD, A: 255})
		}
	}
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("生成测试 PNG 失败: %v", err)
	}
	original := buf.Bytes()
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(original)

	script := `
		var out = resourceAdmit({
			sourceKind: "data",
			source: "` + dataURI + `",
			name: "roundtrip.png",
			kindHint: "I",
			origin: "client-upload",
			retain: "normal",
			dedup: true
		});
		JSON.stringify({ err: out[1] ? String(out[1]) : null, result: out[0] });
	`
	value, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("执行 JS 失败: %v", err)
	}
	payload := value.String()
	if !contains(payload, `"err":null`) {
		t.Fatalf("data URI 入库失败: %s", payload)
	}
	var admitted struct {
		Result struct {
			BlobPath    string `json:"blob_path"`
			ContentHash string `json:"content_hash"`
			Handle      string `json:"handle"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(payload), &admitted); err != nil {
		t.Fatalf("解析入库结果失败: %v（%s）", err, payload)
	}
	blobPath := filepath.Join(*GeneralConfig.LocalDir, filepath.FromSlash(admitted.Result.BlobPath))
	onDisk, err := os.ReadFile(blobPath)
	if err != nil {
		t.Fatalf("读取入库实体失败: %v（%s）", err, blobPath)
	}
	if !bytes.Equal(onDisk, original) {
		diffAt := -1
		for i := 0; i < len(original) && i < len(onDisk); i++ {
			if original[i] != onDisk[i] {
				diffAt = i
				break
			}
		}
		t.Fatalf("%s 实体字节与原图不一致（长度 %d vs %d，首个差异偏移 %d）——"+
			"说明入库链路里有人在 JS 侧解码二进制，字节已被 U+FFFD 替换",
			admitted.Result.Handle, len(original), len(onDisk), diffAt)
	}
	sum := sha256.Sum256(original)
	if want := hex.EncodeToString(sum[:])[:16]; admitted.Result.ContentHash != want {
		t.Errorf("内容哈希不符: 实际 %s，期望 %s", admitted.Result.ContentHash, want)
	}
}

// TestResourceAdmitDedupReusesHandle 断言 dedup=true 时同一内容复用既有句柄、不新增目录行
func TestResourceAdmitDedupReusesHandle(t *testing.T) {
	vm := newBridgeTestRuntime(t)

	script := `
		var first = resourceAdmit({
			sourceKind: "bytes", data: "aGVsbG8gZGVkdXA=", name: "same.png",
			kindHint: "I", origin: "client-upload", retain: "normal", dedup: true
		});
		var second = resourceAdmit({
			sourceKind: "bytes", data: "aGVsbG8gZGVkdXA=", name: "same.png",
			kindHint: "I", origin: "client-upload", retain: "normal", dedup: true
		});
		JSON.stringify({
			err: (first[1] ? String(first[1]) : (second[1] ? String(second[1]) : null)),
			firstHandle: first[0] ? first[0].handle : null,
			secondHandle: second[0] ? second[0].handle : null,
			secondDeduped: second[0] ? second[0].deduped : null
		});
	`
	value, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("执行 JS 失败: %v", err)
	}
	payload := value.String()
	t.Logf("去重结果: %s", payload)

	var got struct {
		Err           *string `json:"err"`
		FirstHandle   string  `json:"firstHandle"`
		SecondHandle  string  `json:"secondHandle"`
		SecondDeduped bool    `json:"secondDeduped"`
	}
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("解析结果失败: %v（%s）", err, payload)
	}
	if got.Err != nil {
		t.Fatalf("入库失败: %s", *got.Err)
	}
	if !got.SecondDeduped {
		t.Errorf("第二次入库应标记为去重命中: %s", payload)
	}
	// 关键性质：同一内容两次入库必须得到同一个句柄（序号由目录状态决定，不写死具体值）
	if got.FirstHandle == "" || got.FirstHandle != got.SecondHandle {
		t.Errorf("同一内容应复用同一句柄: 第一次 %q，第二次 %q", got.FirstHandle, got.SecondHandle)
	}
}

// contains 判断字符串包含关系（避免引入 strings 只为一次判断）
func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

// indexOf 朴素子串查找
func indexOf(haystack, needle string) int {
	if needle == "" {
		return 0
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
