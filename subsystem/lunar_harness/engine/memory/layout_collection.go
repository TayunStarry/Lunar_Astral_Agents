package memory

// layout_collection.go 集合与存储目录辅助

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"os"
)

func (d *Store) collectionByName(id string) *Collection {
	d.collectionsMu.RLock()
	defer d.collectionsMu.RUnlock()
	return d.collections[id]
}

// collectionContaining 返回当前持有指定条目的存储单元（总库或正式簇）
func (d *Store) collectionContaining(docID string) *Collection {
	for _, c := range d.snapshotCollections() {
		c.mu.RLock()
		for i := range c.Documents {
			if c.Documents[i].ID == docID {
				c.mu.RUnlock()
				return c
			}
		}
		c.mu.RUnlock()
	}
	return nil
}

// removeClusterStorage 删除空簇的内存单元与目录
func (d *Store) removeClusterStorage(clusterID string) {
	if clusterID == "" || d.baseDir == "" {
		return
	}
	d.collectionsMu.Lock()
	delete(d.collections, clusterID)
	d.collectionsMu.Unlock()
	if err := os.RemoveAll(clusterDirForID(d.baseDir, clusterID)); err != nil {
		logger.Warn("FileManager", "删除空簇目录 %s 失败: %v", clusterID, err)
	}
}
