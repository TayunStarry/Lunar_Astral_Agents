package memory

// keyword_hooks.go Store 同步钩子

func (d *Store) keywordIndexAdd(c *Collection, doc Document) {
	d.mutateKeywordIndex(func(idx *keywordIndex) { idx.add(doc, c.Name) })
}

func (d *Store) keywordIndexRemove(id string) {
	d.mutateKeywordIndex(func(idx *keywordIndex) { idx.remove(id) })
}

func (d *Store) keywordIndexRemoveDocs(ids []string) {
	d.mutateKeywordIndex(func(idx *keywordIndex) {
		for _, id := range ids {
			idx.remove(id)
		}
	})
}

func (d *Store) keywordIndexSetPending(id string, pending bool) {
	d.mutateKeywordIndex(func(idx *keywordIndex) { idx.setPending(id, pending) })
}

// keywordIndexRelocate 只刷新集合归属；集合字段不落盘，无需重写索引文件。
func (d *Store) keywordIndexRelocate(ids []string, collection string) {
	d.keywordMu.Lock()
	if d.keywordIndex != nil {
		d.keywordIndex.relocate(ids, collection)
	}
	d.keywordMu.Unlock()
}

// refreshKeywordIndexCollections 布局迁移后按当前集合重新映射条目归属（不重建倒排）。
func (d *Store) refreshKeywordIndexCollections() {
	sources := d.keywordIndexSources()
	d.keywordMu.Lock()
	if d.keywordIndex != nil {
		d.refreshKeywordCollections(d.keywordIndex, sources)
	}
	d.keywordMu.Unlock()
}
