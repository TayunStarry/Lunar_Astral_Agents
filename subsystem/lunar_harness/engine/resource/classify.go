package resource

// classify.go 资源类型分类的唯一入口
//
// 新协议下类型完全由发送侧声明：多模态信封的 Kind / 声明 MIME 是唯一依据，
// 后端不再根据 URL 后缀、主机名、扩展名或远程 Content-Type 反推类型。
// 这里只负责把「声明」翻译成内部类型码，不做任何推断。

import (
	"path/filepath"
)

// Classify 按显式声明给出类型码；没有声明时返回空 Kind。
//
// 允许的声明来源：KindHint（信封类型）与 MIME（发送侧声明的媒体类型）。
// 两者都没有时不再兜底猜测，由调用方决定如何处理。
func Classify(in ClassifyInput) ClassifyResult {
	ext := classifyExt(in)
	kind := trimKind(in.KindHint)
	if kind == "" {
		if mimeKind, kindExt := kindFromMIME(in.MIME); mimeKind != "" {
			kind = mimeKind
			if ext == "" {
				ext = kindExt
			}
		}
	}
	if kind == "" {
		return ClassifyResult{Ext: ext}
	}
	return ClassifyResult{
		Kind:  kind,
		Ext:   ext,
		MIME:  mimeForKind(kind, ext),
		Basis: ClassifyBasisDeclared,
	}
}

// classifyExt 取候选扩展名：建议文件名 > 本地路径 > URL 路径。
//
// 扩展名只用于落盘命名与展示，不参与类型判定。
func classifyExt(in ClassifyInput) string {
	if ext := normalizeExt(filepath.Ext(in.Name)); ext != "" {
		return ext
	}
	if in.LocalPath != "" {
		if ext := normalizeExt(filepath.Ext(in.LocalPath)); ext != "" {
			return ext
		}
	}
	if IsRemoteURL(in.Source) {
		return normalizeExt(extOfURL(in.Source))
	}
	if ext := normalizeExt(filepath.Ext(in.Source)); ext != "" {
		return ext
	}
	return normalizeExt(extOfURL(in.Source))
}
