package memory

// keyword_file.go 索引文件读写

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func keywordIndexFileFromIndex(idx *keywordIndex) *keywordIndexFile {
	file := &keywordIndexFile{
		Version:     KeywordIndexVersion,
		GeneratedAt: time.Now().Unix(),
	}
	remap := make(map[uint32]uint32, len(idx.docs))
	for ord, kd := range idx.ords {
		if kd == nil {
			continue
		}
		remap[uint32(ord)] = uint32(len(file.Docs))
		file.Docs = append(file.Docs, keywordDocRecord{
			ID:        kd.ID,
			Type:      kd.Type,
			Timestamp: kd.Timestamp,
			Pending:   kd.Pending,
			Hash:      kd.Hash,
		})
	}

	tokens := make([]string, 0, len(idx.tokens))
	for token := range idx.tokens {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	for _, token := range tokens {
		postings := idx.tokens[token]
		pairs := make([]keywordPosting, 0, len(postings))
		for _, p := range postings {
			newOrd, ok := remap[p.Ord]
			if !ok {
				continue
			}
			pairs = append(pairs, keywordPosting{Ord: newOrd, TF: p.TF})
		}
		if len(pairs) == 0 {
			continue
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].Ord < pairs[j].Ord })
		flattened := make([]uint32, 0, len(pairs)*2)
		for _, p := range pairs {
			flattened = append(flattened, p.Ord, uint32(p.TF))
		}
		file.Tokens = append(file.Tokens, keywordTokenRecord{Token: token, Postings: flattened})
	}
	return file
}

// writeKeywordIndexFile 先写 .tmp 再原子替换；失败时旧索引文件保持不变。
func writeKeywordIndexFile(path string, file *keywordIndexFile) error {
	data, err := json.Marshal(file)
	if err != nil {
		return fmt.Errorf("序列化关键词索引失败: %w", err)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		return fmt.Errorf("压缩关键词索引失败: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("压缩关键词索引失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("写入关键词索引临时文件失败: %w", err)
	}
	if err := substituteRename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("替换关键词索引文件失败: %w", err)
	}
	return nil
}

func loadKeywordIndexFile(path string) (*keywordIndex, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("关键词索引非 gzip 格式: %w", err)
	}
	defer gz.Close()
	data, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("读取关键词索引失败: %w", err)
	}
	var file keywordIndexFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("解析关键词索引失败: %w", err)
	}
	if file.Version != KeywordIndexVersion {
		return nil, fmt.Errorf("关键词索引版本不一致: %d != %d", file.Version, KeywordIndexVersion)
	}

	idx := newKeywordIndex()
	for _, rec := range file.Docs {
		if rec.ID == "" {
			return nil, fmt.Errorf("关键词索引包含空条目 ID")
		}
		if _, dup := idx.docs[rec.ID]; dup {
			return nil, fmt.Errorf("关键词索引条目 ID 重复: %s", rec.ID)
		}
		kd := &keywordDoc{
			ID:        rec.ID,
			Type:      rec.Type,
			Timestamp: rec.Timestamp,
			Pending:   rec.Pending,
			Hash:      rec.Hash,
			Ord:       uint32(len(idx.ords)),
			Terms:     map[string]uint16{},
		}
		idx.ords = append(idx.ords, kd)
		idx.docs[rec.ID] = kd
	}
	for _, rec := range file.Tokens {
		if rec.Token == "" || len(rec.Postings)%2 != 0 {
			return nil, fmt.Errorf("关键词索引倒排记录损坏: %q", rec.Token)
		}
		postings := make([]keywordPosting, 0, len(rec.Postings)/2)
		for i := 0; i < len(rec.Postings); i += 2 {
			ord := rec.Postings[i]
			tf := rec.Postings[i+1]
			if int(ord) >= len(idx.ords) || tf == 0 {
				return nil, fmt.Errorf("关键词索引倒排序号越界: token=%q ord=%d", rec.Token, ord)
			}
			if _, dup := idx.ords[ord].Terms[rec.Token]; dup {
				return nil, fmt.Errorf("关键词索引倒排记录重复: token=%q ord=%d", rec.Token, ord)
			}
			idx.ords[ord].Terms[rec.Token] = uint16(tf)
			postings = append(postings, keywordPosting{Ord: ord, TF: uint16(tf)})
		}
		idx.tokens[rec.Token] = postings
	}
	return idx, nil
}

// ===== 写路径同步钩子 =====
