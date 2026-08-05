# 数据模型

## 1. 关系概览

```text
User
├── UserSession[]
└── UserIdentity[]
```

当前认证范围不增加组织、项目和项目成员关系。

## 2. users

建议字段：

- `id`
- `username`，唯一
- `display_name`
- `password_hash`，可空；仅使用 OIDC 的账号不保存本地密码
- `role`，`admin` 或 `member`
- `status`，`active` 或 `disabled`
- `last_login_at`
- `created_at`
- `updated_at`

约束：系统至少保留一个启用状态的管理员，不能通过普通管理操作删除或停用最后一个管理员。

## 3. user_sessions

建议字段：

- `id`
- `user_id`
- `token_hash`
- `expires_at`
- `revoked_at`
- `created_at`
- `last_seen_at`

服务端只保存会话 Token 哈希，不保存浏览器可直接使用的明文会话 Token。

## 4. user_identities

- `id`
- `user_id`
- `issuer`
- `subject`
- `email`，仅作为身份元数据
- `created_at`
- `updated_at`

`issuer + subject` 唯一确定一个外部身份，`user_id + issuer` 也具有唯一约束。系统不按
邮箱自动合并本地账号，OIDC access token、ID Token 和 Client Secret 不写入该表。

## 5. system_settings

当前仅保存 `initial_admin_created` 安装标记。该表不保存初始化令牌、密码、连接信息或
其他部署密钥。标记与第一个管理员在同一数据库事务中创建，保证初始化只能完成一次。

## 6. 当前迁移状态

`0001_users_and_sessions.sql` 创建用户和会话；
`0002_registration_oidc_and_setup.sql` 将本地密码改为可空，并创建外部身份与安装标记。
迁移由显式命令运行，不在服务启动时自动执行。

以下钱包和 API Key 模型仍属于后续路线图，当前没有创建表或空壳代码。

## 7. wallets

每个用户一个钱包，建议字段：

- `id`
- `user_id`，唯一
- `balance`
- `status`
- `created_at`
- `updated_at`

余额使用定点数类型，避免使用浮点数。钱包余额和流水变更需要在同一事务中完成。

## 8. wallet_ledger_entries

建议字段：

- `id`
- `wallet_id`
- `amount`
- `balance_after`
- `entry_type`，例如 `manual_adjustment`、`usage`、`refund`
- `reference_id`
- `description`
- `created_by`
- `created_at`

实现余额功能时保留流水表，方便审计、人工调整和后续接入计费。

## 9. api_keys

建议字段：

- `id`
- `user_id`
- `name`
- `key_prefix`
- `key_hash`
- `status`，`active` 或 `revoked`
- `last_used_at`
- `created_at`
- `revoked_at`

完整 Key 只在创建成功时显示一次。数据库只保存哈希和前缀。Key 前缀用于用户识别自己的 Key，但不能用于认证。

## 10. 数据库扩展

使用 Ent 生成数据访问层，以 MySQL 为默认数据库，同时避免在业务代码中散落只适用于
MySQL 的 SQL，让后续更换数据库时只需要替换驱动和迁移细节。
