package forum

// migrate.go 账号文件的幂等迁移。
//
// 迁移口径（v2）：
//   1. 删除服务端已废弃的 model 字段（JSON 解码后不再有该字段，重新序列化即消失）；
//   2. 写入 schema_version=2；
//   3. 新增 qq_masked / avatar_url / avatar_fallback / kind / test_mode 等公开身份字段（缺省为空）；
//   4. 覆盖写之前自动把旧文件复制到 forum_account.json.bak，回滚只需把 .bak 复制回原路径。
//
// 幂等性：第二次读取时 schema_version 已是 2 且不再含 model 键，函数直接返回，不重写文件，
// 因此文件的字节内容与 UpdatedAt 都保持不变。

import (
	"encoding/json"
)

// migrateAccountFile 检测并迁移旧版账号文件；无可迁移内容时不写盘。
func migrateAccountFile(path string, raw []byte, acct *Account) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	_, hasLegacyModel := fields["model"]
	if acct.SchemaVersion >= AccountSchemaVersion && !hasLegacyModel {
		return nil
	}
	acct.SchemaVersion = AccountSchemaVersion
	return writeAccountFile(path, acct)
}
