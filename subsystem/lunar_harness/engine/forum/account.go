package forum

// account.go 本地账号文件读写：原子落盘、覆盖前 .bak 回滚副本、v2 幂等迁移与写互斥。
//
// 磁盘格式与迁移口径完全沿用 agents/MutualForum 历史实现：
//   - 文件路径、字段名与顺序、.bak 命名（<path>.bak）均不变；
//   - 旧结构在读取时原位迁移为 schema_version=2，重复读取不再写盘；
//   - 每次覆盖写入前把上一份内容复制到 .bak，回滚只需复制回原路径。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// LoadAccount 读取本地账号文件；文件不存在或服务地址已变化时返回 nil。
// 读到旧结构时会在同一次读取中做幂等迁移（写失败不阻塞使用，仅记录告警）。
func LoadAccount(path, endpoint string) (*Account, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, ErrAccountPathEmpty
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取互助论坛账号文件失败: %w", err)
	}
	var acct Account
	if err := json.Unmarshal(raw, &acct); err != nil {
		return nil, fmt.Errorf("解析互助论坛账号文件失败: %w", err)
	}
	if migErr := migrateAccountFile(path, raw, &acct); migErr != nil {
		logger.Warn("LunarMatrix", "迁移互助论坛账号文件失败（本次仍按内存结构使用）: %v", migErr)
	}
	if acct.Endpoint != "" && acct.Endpoint != endpoint {
		// 服务地址换过时旧身份不沿用，避免把 A 站凭证发到 B 站
		return nil, nil
	}
	acct.Endpoint = endpoint
	return &acct, nil
}

// SaveAccount 写入账号文件：补齐 schema 版本与更新时间后原子落盘。
func SaveAccount(path string, acct *Account) error {
	if acct == nil {
		return ErrAccountNil
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return ErrAccountPathEmpty
	}
	acct.SchemaVersion = AccountSchemaVersion
	acct.UpdatedAt = time.Now().Unix()
	return writeAccountFile(path, acct)
}

// writeAccountFile 原子写入账号文件，并在覆盖前保留上一份 .bak（Windows 下权限依赖用户目录 ACL）。
func writeAccountFile(path string, acct *Account) error {
	accountWriteMu.Lock()
	defer accountWriteMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建互助论坛账号目录失败: %w", err)
	}
	if previous, err := os.ReadFile(path); err == nil {
		if backupErr := os.WriteFile(path+AccountBackupSuffix, previous, 0o600); backupErr != nil {
			return fmt.Errorf("备份互助论坛账号文件失败: %w", backupErr)
		}
	}
	raw, err := json.MarshalIndent(acct, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化互助论坛账号失败: %w", err)
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, raw, 0o600); err != nil {
		return fmt.Errorf("写入互助论坛账号失败: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("保存互助论坛账号失败: %w", err)
	}
	return nil
}
