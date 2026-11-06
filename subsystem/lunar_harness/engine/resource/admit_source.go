package resource

// admit_source.go 入库来源归一：本地路径 / 工作区 / 下载件 / data URI / 字节流，
// 全部收敛为「一个本地文件 + 建议文件名」；临时文件由调用方负责清理。

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/netsource"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// normalizeAdmitSource 将来源归一为本地文件路径；cleanup 非空时由调用方负责清理
func normalizeAdmitSource(in AdmitInput) (string, func(), error) {
	switch in.SourceKind {
	case SourceKindPath, "":
		local, err := resolveLocalSource(in.Source)
		if err != nil {
			return "", nil, err
		}
		if in.Name == "" {
			in.Name = filepath.Base(local)
		}
		return local, nil, nil
	case SourceKindURL:
		// 本机服务地址（含手机端经局域网 IP / 自签 HTTPS 访问的 origin）直接落到本地文件，
		// 不回源 HTTP：回源会因证书校验失败而丢弃已经落盘的资源。
		if local, err := resolveLocalSource(in.Source); err == nil {
			if in.Name == "" {
				in.Name = filepath.Base(local)
			}
			return local, nil, nil
		}
		return downloadToTemp(in.Source, in.Name)
	case SourceKindData:
		data, err := decodeDataURI(in.Source)
		if err != nil {
			return "", nil, err
		}
		return writeTemp(data, in.Name)
	case SourceKindBytes:
		if len(in.Data) == 0 {
			return "", nil, fmt.Errorf("字节来源为空")
		}
		return writeTemp(in.Data, in.Name)
	case SourceKindWorkspace:
		return resolveWorkspaceSource(in.Source)
	default:
		return "", nil, fmt.Errorf("不支持的来源类型: %q", in.SourceKind)
	}
}

// resolveWorkspaceSource 解析用户直接引用的工作区文件
//
// 与 LocalDir 来源的区别：LocalDir 存的是系统自己的数据（上传件、媒体），
// 而工作区是用户的项目目录（源码、文档）。两类都要能在对话里被引用，
// 但工作区文件按只读方式收录，且限制在文本类扩展名内。
//
// 安全约束：
//   - 相对路径一律以进程工作目录为根（服务端与工作区同根启动）
//   - 解析后的绝对路径必须仍位于工作目录之下（防 ../ 穿越）
//   - 扩展名必须在 workspaceTextExtensions 白名单内
//   - 体积不得超过 workspaceMaxBytes
func resolveWorkspaceSource(source string) (string, func(), error) {
	rel := strings.ReplaceAll(strings.TrimSpace(source), "\\", "/")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return "", nil, fmt.Errorf("工作区来源为空")
	}
	if after, ok := strings.CutPrefix(rel, "file://"); ok {
		rel = after
	}
	root, err := os.Getwd()
	if err != nil {
		return "", nil, fmt.Errorf("获取工作目录失败: %v", err)
	}
	root = filepath.Clean(root)

	abs := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	// 穿越校验：解析结果必须仍在工作目录之下
	if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return "", nil, fmt.Errorf("工作区路径越界: %s", source)
	}
	// 扩展名白名单（.gitignore / .env 这类无基名的点文件按整体名匹配）
	lowerBase := strings.ToLower(filepath.Base(abs))
	ext := strings.ToLower(filepath.Ext(lowerBase))
	if !workspaceTextExtensions[ext] && !workspaceTextExtensions[lowerBase] {
		return "", nil, fmt.Errorf("工作区引用仅支持文本类文件，不支持: %s", lowerBase)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", nil, fmt.Errorf("工作区文件不存在: %s", source)
	}
	if info.IsDir() {
		return "", nil, fmt.Errorf("工作区引用不支持目录: %s", source)
	}
	if info.Size() > workspaceMaxBytes {
		return "", nil, fmt.Errorf("工作区文件过大（%d 字节，上限 %d）: %s", info.Size(), int64(workspaceMaxBytes), source)
	}
	return abs, nil, nil
}

// resolveLocalSource 把各种本地/同源表示解析为可读的本地文件路径
//
// 支持：绝对路径、相对 LocalDir 的路径、file:// 前缀、本机 http(s) 服务地址（/file/read/...）
func resolveLocalSource(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("来源为空")
	}
	// file:// 前缀
	if rest, ok := strings.CutPrefix(source, "file://"); ok {
		source = rest
	}
	// 本机服务地址：前端上传后的图片以 http(s)://本机地址/... 形式进入
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		if rel, ok := netsource.FileReadRel(source); ok {
			source = rel
		} else {
			return "", fmt.Errorf("非本机地址请使用 url 来源: %s", source)
		}
	}
	// 本机服务路由的裸路径：历史对话缓存里存的是 `/file/read/multimedia/...`，
	// 它不是 LocalDir 下的真实子目录。必须先剥掉路由前缀，否则会被拼成
	// `local_data/file/read/multimedia/...`，导致明明存在的文件被判为「本地来源不存在」。
	if rel, ok := netsource.FileReadRel(source); ok {
		source = rel
	}
	abs, err := resolveUnderLocalDir(source)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("本地来源不存在: %s", source)
	}
	return abs, nil
}

// localPathFromServiceURL 从本机服务地址中提取相对 LocalDir 的路径
//
// 前端上传后返回 `${origin}/file/read/<relative>`，其中 relative 即 LocalDir 下的路径。
// origin 可能是回环地址，也可能是手机端的局域网访问地址（自签 HTTPS 代理）；
// 两者指向的都是本机服务，必须直接映射到本地文件。
func localPathFromServiceURL(raw string) (string, bool) {
	return netsource.FileReadRel(raw)
}

// resolveUnderLocalDir 解析相对 LocalDir 的路径并做穿越校验
func resolveUnderLocalDir(rel string) (string, error) {
	return netsource.UnderLocalDir(rel)
}

// sniffFileImageFormat 读取文件头识别图片真实格式；无法识别时返回空串。
//
// 只读 32 字节：主流图片格式的魔数都在文件最前部，避免为嗅探把整个大图读入内存。
func sniffFileImageFormat(localPath string) (ext string, mimeType string) {
	f, err := os.Open(localPath)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	head := make([]byte, 32)
	n, _ := io.ReadFull(f, head)
	if n == 0 {
		return "", ""
	}
	return persistence.SniffImageFormat(head[:n])
}

// writeTemp 将字节写入临时文件，返回路径与清理函数
func writeTemp(data []byte, name string) (string, func(), error) {
	ext := filepath.Ext(name)
	tmp, err := os.CreateTemp("", "lunar-admit-*"+ext)
	if err != nil {
		return "", nil, fmt.Errorf("创建临时文件失败: %v", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", nil, fmt.Errorf("写入临时文件失败: %v", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", nil, fmt.Errorf("关闭临时文件失败: %v", err)
	}
	return tmp.Name(), func() { os.Remove(tmp.Name()) }, nil
}

// downloadToTemp 下载远程地址到临时文件，返回路径与清理函数
func downloadToTemp(raw string, name string) (string, func(), error) {
	return downloadToTempLimited(raw, name, maxRemoteResourceBytes)
}

// downloadToTempLimited 按指定大小上限回源下载（上限抽参便于测试）
func downloadToTempLimited(raw string, name string, maxBytes int64) (string, func(), error) {
	// 腾讯图床只接受 https：http 直链会被边缘节点直接拒绝（400）。
	raw = UpgradeCDNScheme(raw)
	resp, err := netsource.Open(raw, remoteResourceTimeout)
	if err != nil {
		return "", nil, fmt.Errorf("下载资源失败: %v", err)
	}
	defer resp.Body.Close()

	ext := filepath.Ext(name)
	if ext == "" {
		ext = filepath.Ext(parseURLPath(raw))
	}
	tmp, err := os.CreateTemp("", "lunar-admit-*"+ext)
	if err != nil {
		return "", nil, fmt.Errorf("创建临时文件失败: %v", err)
	}
	cleanup := func() { os.Remove(tmp.Name()) }
	if err := netsource.CopyLimited(tmp, resp.Body, maxBytes); err != nil {
		tmp.Close()
		cleanup()
		return "", nil, fmt.Errorf("下载写入失败: %v", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("关闭临时文件失败: %v", err)
	}
	return tmp.Name(), cleanup, nil
}

// parseURLPath 提取 URL 的路径部分（失败时返回空串）
func parseURLPath(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Path
}

// decodeDataURI 解码 data:<mime>;base64,<payload> 形态的内联数据
func decodeDataURI(raw string) ([]byte, error) {
	if !strings.HasPrefix(raw, "data:") {
		return nil, fmt.Errorf("非法的 data URI")
	}
	before, after, ok := strings.Cut(raw, ",")
	if !ok {
		return nil, fmt.Errorf("非法的 data URI：缺少数据分隔符")
	}
	meta, payload := before, after
	if !strings.Contains(meta, "base64") {
		decoded, err := url.PathUnescape(payload)
		if err != nil {
			return nil, fmt.Errorf("解码 data URI 失败: %v", err)
		}
		return []byte(decoded), nil
	}
	data, err := base64Decode(payload)
	if err != nil {
		return nil, fmt.Errorf("解码 data URI 失败: %v", err)
	}
	return data, nil
}
