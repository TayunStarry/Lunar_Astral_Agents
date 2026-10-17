//go:build windows

package TuiSystem

// ==== 进程级标准流接管（原生代码侧） ====
//
// 为什么会漏：Install 换的只是 Go 层的 os.Stdout / os.Stderr / log 输出，**进程级标准句柄没动**。
// 进程内加载的原生库用的是 CRT 的 printf/fprintf —— 典型是 OmniVoice TTS 引擎（以 DLL 形式
// 经 LoadLibraryExW 进来，源码在 engine/omnivoice_tts/cpp/src）：
//   - maskgit-tts.h 每步打印 `[MaskGIT-Step] 25/32 demask=12 remaining=239`；
//   - backend.h 的 ggml 日志桥 `fputs(text, stderr)`；
//   - bpe.h / gguf-weights.h / dac-*.h / hubert-enc.h 的加载信息与告警。
// 这些字节直写 STD_ERROR_HANDLE 指向的控制台屏幕缓冲区，正好落在 TUI 帧上（实测：合成
// 语音时界面被进度行逐行刷乱）。按设计，这类引擎诊断只应出现在「原始日志」视图里。
//
// 修法：把标准句柄槽（STD_OUTPUT/STD_ERROR）与 CRT 文件描述符 fd 1/2 一并重定向到日志管道，
// 原生输出于是与 Go 输出同一口径——经 scanPipeLines 只进原始日志与 console.log。
//
// 覆盖边界：
//   - CRT 标准流（printf/fprintf/std::cout；msvcrt.dll 与 ucrtbase.dll 两套运行时都试）；
//   - 经 GetStdHandle 取标准句柄的写入；
//   - **不**覆盖直接 CreateFile("CONOUT$") 的写入（那不是标准流通道，引擎侧未使用）。
//
// 实测结论（探针复现，两步都不可少）：
//   - 只 SetStdHandle 拦不住 fprintf：CRT 的文件描述符表在**运行时 DLL 加载时**就已把 fd 1/2
//     绑到当时的继承句柄上，动态加载的引擎 DLL 与宿主共用同一张表，改「标准句柄槽」对它无效；
//   - 因此还要把 fd 1/2 重定向，且 _dup2 只认**文件描述符**：必须先用 _open_osfhandle 把
//     「管道句柄副本」变成临时 fd，再 _dup2(srcFd, 1/2)；直接把句柄塞给 _dup2 一定失败，
//     而且返回寄存器高位带垃圾会让 `== -1` 误判成功。
//
// 前提：调用方须先确保渲染目标是本进程自取的句柄（resolveRenderTarget 已保证）——
// 接管会关掉 CRT 表里原有的继承句柄，界面若正用着它就会失去输出。

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// crtBinaryFlag 即 _O_BINARY：管道按二进制写，不做 LF→CRLF 翻译，原生进度行 (\r) 原样保留
const crtBinaryFlag = 0x8000

// crtRuntimeDLLs 持有 CRT 文件描述符表的运行时 DLL（机器上没有的会跳过）。
// MinGW 构建的引擎 DLL 走 msvcrt.dll，UCRT 工具链构建的走 ucrtbase.dll，
// 同一进程里两种都可能出现，因此都要处理。
var crtRuntimeDLLs = []string{"msvcrt.dll", "ucrtbase.dll"}

// captureNativeStdio 把进程级标准输出/错误接到 w（日志管道的写端），
// 返回未接上的通道说明；全部接上时返回空串。
func captureNativeStdio(w *os.File) string {
	if w == nil {
		return "日志管道不可用"
	}
	handle := windows.Handle(w.Fd())
	var failed []string

	// 1) 标准句柄槽：此后 GetStdHandle(STD_*) 取到的即是管道（覆盖运行期自行取标准句柄的写入）
	for _, slot := range []uint32{windows.STD_OUTPUT_HANDLE, windows.STD_ERROR_HANDLE} {
		if err := windows.SetStdHandle(slot, handle); err != nil {
			failed = append(failed, stdSlotName(slot))
		}
	}

	// 2) CRT 的 fd 表：标准流本体（printf/fprintf/cout）走这里，必须补这一步
	for _, fd := range []int{1, 2} {
		if !crtRedirect(fd, handle) {
			failed = append(failed, fmt.Sprintf("CRT fd %d", fd))
		}
	}

	if len(failed) == 0 {
		return ""
	}
	return "未接上 " + strings.Join(failed, "、")
}

// crtRedirect 把指定 CRT 文件描述符重定向到 handle（任一运行时接上即算成功）。
//
// 每个运行时各走一遍「建 fd → 改 fd」：_open_osfhandle(句柄副本) → _dup2(srcFd, fd)。
//   - 句柄用**独立副本**：_dup2 会关掉目标 fd 原先绑定的句柄，若两个运行时共用同一句柄值，
//     先接上的那个会把后一个正要关的句柄一并关掉；副本让它们互不干扰，也不牵连 Go 侧管道句柄。
//   - srcFd **刻意不关**：_dup2 后它与目标 fd 共用同一句柄，关掉会让目标 fd 一起失效。
//     所有权交给该运行时的 CRT，进程退出时由它回收。
func crtRedirect(fd int, handle windows.Handle) bool {
	patched := false
	for _, dll := range crtRuntimeDLLs {
		runtime := syscall.NewLazyDLL(dll)
		openOSFHandle := runtime.NewProc("_open_osfhandle")
		dup2 := runtime.NewProc("_dup2")
		if openOSFHandle.Find() != nil || dup2.Find() != nil {
			continue // 该运行时不在本机 / 未导出所需符号，换下一个
		}
		srcHandle, ok := duplicateHandle(handle)
		if !ok {
			continue
		}
		// 失败返回 -1；必须按 32 位取低半，直接用 int(ret) 比 -1 会被高位垃圾判成成功
		srcFd, _, _ := openOSFHandle.Call(uintptr(srcHandle), uintptr(crtBinaryFlag))
		if int32(srcFd) == -1 {
			_ = windows.CloseHandle(srcHandle) // 未被 CRT 接管，自己关掉
			continue
		}
		if ret, _, _ := dup2.Call(srcFd, uintptr(fd)); int32(ret) != -1 {
			patched = true
			continue
		}
		// _dup2 失败：临时 fd 无人使用，交回该运行时回收（连带关闭句柄副本）
		if closeProc := runtime.NewProc("_close"); closeProc.Find() == nil {
			closeProc.Call(srcFd)
		}
	}
	return patched
}

// duplicateHandle 复制一个可继承的句柄副本（失败返回 false）
func duplicateHandle(h windows.Handle) (windows.Handle, bool) {
	current := windows.CurrentProcess()
	var dup windows.Handle
	if err := windows.DuplicateHandle(current, h, current, &dup, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return 0, false
	}
	return dup, true
}

// stdSlotName 标准句柄槽的可读名（仅用于诊断留痕）
func stdSlotName(slot uint32) string {
	if slot == windows.STD_ERROR_HANDLE {
		return "STD_ERROR_HANDLE"
	}
	return "STD_OUTPUT_HANDLE"
}
