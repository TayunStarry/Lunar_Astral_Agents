package memory

// keyword_index.go 倒排索引核心操作

func newKeywordIndex() *keywordIndex {
	return &keywordIndex{
		docs:   make(map[string]*keywordDoc),
		tokens: make(map[string][]keywordPosting),
	}
}

func (idx *keywordIndex) allocOrd() uint32 {
	if n := len(idx.free); n > 0 {
		ord := idx.free[n-1]
		idx.free = idx.free[:n-1]
		return ord
	}
	ord := uint32(len(idx.ords))
	idx.ords = append(idx.ords, nil)
	return ord
}

// add 新增或更新一条记忆（同 ID 先撤销旧倒排，再按新正文重建）。
func (idx *keywordIndex) add(doc Document, collection string) {
	if _, ok := idx.docs[doc.ID]; ok {
		idx.remove(doc.ID)
	}
	ord := idx.allocOrd()
	kd := &keywordDoc{
		ID:         doc.ID,
		Type:       documentType(doc),
		Timestamp:  doc.Timestamp,
		Pending:    doc.Pending,
		Hash:       keywordContentHash(doc.Content),
		Collection: collection,
		Ord:        ord,
		Terms:      keywordTokenize(doc.Content),
	}
	idx.docs[doc.ID] = kd
	idx.ords[ord] = kd
	for token, tf := range kd.Terms {
		idx.tokens[token] = append(idx.tokens[token], keywordPosting{Ord: ord, TF: tf})
	}
}

// remove 撤销一条记忆的全部倒排记录。
func (idx *keywordIndex) remove(id string) {
	kd, ok := idx.docs[id]
	if !ok {
		return
	}
	for token := range kd.Terms {
		postings := idx.tokens[token]
		out := postings[:0]
		for _, p := range postings {
			if p.Ord != kd.Ord {
				out = append(out, p)
			}
		}
		if len(out) == 0 {
			delete(idx.tokens, token)
		} else {
			idx.tokens[token] = out
		}
	}
	delete(idx.docs, id)
	idx.ords[kd.Ord] = nil
	idx.free = append(idx.free, kd.Ord)
}

// relocate 刷新条目所属集合（条目搬迁不改变正文与词元，无需重建倒排）。
func (idx *keywordIndex) relocate(ids []string, collection string) {
	for _, id := range ids {
		if kd, ok := idx.docs[id]; ok {
			kd.Collection = collection
		}
	}
}

func (idx *keywordIndex) setPending(id string, pending bool) {
	if kd, ok := idx.docs[id]; ok {
		kd.Pending = pending
	}
}

func (idx *keywordIndex) postingCount() int {
	n := 0
	for _, postings := range idx.tokens {
		n += len(postings)
	}
	return n
}

// ===== 构建 / 加载 / 持久化 =====
