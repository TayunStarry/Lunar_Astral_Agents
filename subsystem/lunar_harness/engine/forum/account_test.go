package forum

// account_test.go 账号文件持久化：v2 幂等迁移、.bak 回滚副本与并发写互斥。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestAccountMigrationRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "forum_account.json")
	endpoint := "https://forum.example.com/api/forum/v1"

	legacy := map[string]any{
		"schema_version":       1,
		"endpoint":             endpoint,
		"instance_uid":         "lh_legacy",
		"name":                 "月华·旧版",
		"model":                "legacy-model",
		"api_key":              "lhf_legacy",
		"round_id":             "r_legacy",
		"round_issued_at":      100,
		"last_seq":             42,
		"event_seq":            7,
		"min_interval_seconds": 30,
		"round_ttl_seconds":    600,
		"validated_at":         200,
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("构造旧账号失败: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("写入旧账号失败: %v", err)
	}

	acct, err := LoadAccount(path, endpoint)
	if err != nil {
		t.Fatalf("读取旧账号失败: %v", err)
	}
	if acct == nil || acct.SchemaVersion != AccountSchemaVersion {
		t.Fatalf("迁移后 schema_version 应为 %d，实际 %+v", AccountSchemaVersion, acct)
	}
	if acct.APIKey != "lhf_legacy" || acct.LastSeq != 42 || acct.EventSeq != 7 {
		t.Fatalf("迁移不应丢失凭证或游标: %+v", acct)
	}

	migrated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取迁移结果失败: %v", err)
	}
	if strings.Contains(string(migrated), `"model"`) {
		t.Fatalf("迁移后不应残留 model 字段: %s", migrated)
	}

	backup, err := os.ReadFile(path + AccountBackupSuffix)
	if err != nil {
		t.Fatalf("迁移必须保留 .bak 备份: %v", err)
	}
	if string(backup) != string(raw) {
		t.Fatalf(".bak 应为迁移前原始字节\n期望: %s\n实际: %s", raw, backup)
	}

	// 第二次读取必须幂等：不重写文件，字节内容保持不变。
	if _, err := LoadAccount(path, endpoint); err != nil {
		t.Fatalf("二次读取失败: %v", err)
	}
	afterSecond, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("二次读取账号文件失败: %v", err)
	}
	if string(afterSecond) != string(migrated) {
		t.Fatalf("重复执行迁移不应改写文件\n期望: %s\n实际: %s", migrated, afterSecond)
	}
}

func TestSaveAccountKeepsPreviousBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "forum_account.json")
	endpoint := "https://forum.example.com/api/forum/v1"

	first := &Account{Endpoint: endpoint, InstanceUID: "lh_backup", APIKey: "lhf_first"}
	if err := SaveAccount(path, first); err != nil {
		t.Fatalf("首次保存失败: %v", err)
	}
	second := *first
	second.APIKey = "lhf_second"
	if err := SaveAccount(path, &second); err != nil {
		t.Fatalf("二次保存失败: %v", err)
	}

	backup, err := os.ReadFile(path + AccountBackupSuffix)
	if err != nil {
		t.Fatalf("覆盖前必须写 .bak: %v", err)
	}
	if !strings.Contains(string(backup), "lhf_first") {
		t.Fatalf(".bak 应保留上一份凭证: %s", backup)
	}
	stored := readAccountFileForTest(t, path)
	if stored.APIKey != "lhf_second" || stored.SchemaVersion != AccountSchemaVersion {
		t.Fatalf("当前账号文件异常: %+v", stored)
	}
}

// TestLegacyAccountReadThenWriteStaysV2 旧格式读入迁移后，再写回仍是 v2，且只保留一份 .bak。
func TestLegacyAccountReadThenWriteStaysV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "forum_account.json")
	endpoint := "https://forum.example.com/api/forum/v1"

	legacy, err := json.Marshal(map[string]any{
		"schema_version": 1,
		"endpoint":       endpoint,
		"instance_uid":   "lh_legacy",
		"name":           "月华·旧版",
		"model":          "legacy-model",
		"api_key":        "lhf_legacy",
		"last_seq":       42,
	})
	if err != nil {
		t.Fatalf("构造旧账号失败: %v", err)
	}
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatalf("写入旧账号失败: %v", err)
	}

	acct, err := LoadAccount(path, endpoint)
	if err != nil || acct == nil {
		t.Fatalf("读取旧账号失败: acct=%+v err=%v", acct, err)
	}
	acct.LastSeq = 99
	if err := SaveAccount(path, acct); err != nil {
		t.Fatalf("写回账号失败: %v", err)
	}

	for _, name := range []string{path, path + AccountBackupSuffix} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", name, err)
		}
		if strings.Contains(string(raw), `"model"`) {
			t.Fatalf("%s 不应残留 model 字段: %s", name, raw)
		}
		var decoded Account
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("解析 %s 失败: %v", name, err)
		}
		if decoded.SchemaVersion != AccountSchemaVersion {
			t.Fatalf("%s 应保持 schema_version=%d，实际 %+v", name, AccountSchemaVersion, decoded)
		}
	}
	// .bak 只保留一份：不应出现 .bak.bak 这类二次备份。
	if _, err := os.Stat(path + AccountBackupSuffix + AccountBackupSuffix); !os.IsNotExist(err) {
		t.Fatalf("不应生成 .bak.bak（err=%v）", err)
	}
	backups, err := filepath.Glob(path + AccountBackupSuffix + "*")
	if err != nil {
		t.Fatalf("枚举备份失败: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("应只有一份 .bak，实际 %v", backups)
	}
}

// TestConcurrentSavesKeepFileValid 并发写入应被串行化，最终文件始终可解析且只有一份 .bak。
func TestConcurrentSavesKeepFileValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "forum_account.json")
	endpoint := "https://forum.example.com/api/forum/v1"

	var group sync.WaitGroup
	for index := 0; index < 8; index++ {
		group.Add(1)
		go func(key string) {
			defer group.Done()
			acct := &Account{Endpoint: endpoint, InstanceUID: "lh_concurrent", APIKey: key}
			if err := SaveAccount(path, acct); err != nil {
				t.Errorf("并发保存失败: %v", err)
			}
		}(strings.Repeat("k", index+1))
	}
	group.Wait()

	stored := readAccountFileForTest(t, path)
	if stored.APIKey == "" || stored.SchemaVersion != AccountSchemaVersion {
		t.Fatalf("并发写入后账号文件异常: %+v", stored)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("不应遗留临时文件（err=%v）", err)
	}
}

func readAccountFileForTest(t *testing.T, path string) Account {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取账号文件失败: %v", err)
	}
	var acct Account
	if err := json.Unmarshal(raw, &acct); err != nil {
		t.Fatalf("解析账号文件失败: %v", err)
	}
	return acct
}
