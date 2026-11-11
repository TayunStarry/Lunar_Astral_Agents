package logbuf

// meterstore.go 模型调用计量的持久化。
//
// 计量数据原本只存在于进程内，重启即归零。这里把它落到
// <LocalDir>/database/model_stats.json：
//   - 启动时 LoadModelStats 读回上次累计值，界面接着往上累加；
//   - 每次入账后按 meterSaveInterval 节流写盘，意外关闭最多丢一个间隔；
//   - 正常退出由 GracefulShutdown 调用 SaveModelStats 立即写盘。
//
// 写入采用「临时文件 + 改名」的原子替换，避免写到一半被中断留下半截 JSON。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// LoadModelStats 读取上次累计的计量数据；文件不存在按首次启动处理。
func LoadModelStats() error {
	data, err := os.ReadFile(meterFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var stored ModelCallStats
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	meterMu.Lock()
	meterStats = stored
	meterDirty = false
	meterLastSave = time.Now()
	meterMu.Unlock()
	return nil
}

// SaveModelStats 立即落盘；没有新数据时不重复写。
func SaveModelStats() error {
	meterMu.Lock()
	if !meterDirty {
		meterMu.Unlock()
		return nil
	}
	snapshot := meterStats
	meterDirty = false
	meterLastSave = time.Now()
	meterMu.Unlock()

	if err := writeMeterFile(snapshot); err != nil {
		// 写失败保留脏标记，下一次入账或退出时重试，不让累计值悄悄丢掉。
		meterMu.Lock()
		meterDirty = true
		meterMu.Unlock()
		return err
	}
	return nil
}

// scheduleMeterSave 入账后按间隔触发后台落盘；已有任务或未到间隔时直接返回。
func scheduleMeterSave() {
	meterMu.Lock()
	if meterSaving || !meterDirty || time.Since(meterLastSave) < meterSaveInterval {
		meterMu.Unlock()
		return
	}
	meterSaving = true
	meterMu.Unlock()

	go func() {
		defer func() {
			meterMu.Lock()
			meterSaving = false
			meterMu.Unlock()
		}()
		// 自动落盘失败不阻塞推理：脏标记已由 SaveModelStats 恢复，后续会重试。
		_ = SaveModelStats()
	}()
}

// meterFilePath 计量文件路径（LocalDir 变化时自动跟随）。
func meterFilePath() string {
	return filepath.Join(filepath.Clean(config.LocalDir), "database", meterFileName)
}

// writeMeterFile 原子写入：先写同目录临时文件，再改名覆盖目标。
func writeMeterFile(stats ModelCallStats) error {
	path := meterFilePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, "model_stats-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}
