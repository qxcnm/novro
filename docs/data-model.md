# 数据模型

## 1. 关系概览

```text
User
├── UserSession[]
├── UserIdentity[]
├── APIKey[]
├── Wallet
└── APIUsage[]

Provider
└── ModelRoute[]

Wallet
└── WalletEntry[]
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

约束：系统至少保留一个启用状态的管理员，不能通过角色降级或状态变更移除最后一个
启用的管理员。该约束由服务端在数据库事务内执行。

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
`0002_registration_oidc_and_setup.sql` 将本地密码改为可空，并创建外部身份与安装标记；
`0003_api_keys.sql` 创建用户 API Key；`0004_providers.sql` 创建加密提供商配置；
`0005_wallets_model_routes_and_usage.sql` 创建钱包、流水、模型路由和调用用量；
`0006_idempotent_wallet_entries.sql` 为调用预占和退款增加幂等唯一约束。
迁移由显式命令运行，不在服务启动时自动执行。

## 7. wallets

每个用户一个钱包，建议字段：

- `id`
- `user_id`，唯一
- `balance_micros`，非负整数，1 元人民币等于 1,000,000 微元
- `created_at`
- `updated_at`

余额使用定点数类型，避免使用浮点数。钱包余额和流水变更需要在同一事务中完成。

## 8. wallet_entries

建议字段：

- `id`
- `wallet_id`
- `amount_micros`
- `balance_after_micros`
- `entry_type`，`manual_adjustment`、`usage_reservation` 或 `usage_refund`
- `reference_id`
- `description`
- `actor_user_id`，人工调整时记录管理员
- `created_at`

余额能力保留流水表，方便审计、人工调整和调用结算。

调用开始时先写入负的 `usage_reservation`，成功后写入释放差额的 `usage_refund`；两者的
净变化就是最终调用费用。钱包行使用 `SELECT ... FOR UPDATE` 语义锁定。同一钱包的
`reference_id + entry_type` 唯一，使余额预占和退款可以安全重试；相同金额的重复请求
视为已完成，不同金额的重复请求视为冲突。

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

## 10. providers

- `id`、`code`、`display_name`
- `protocol`，`openai` 或 `anthropic`
- `base_url`
- `encrypted_api_key`，AES-256-GCM 密文，仅服务端读取
- `api_key_hint`，只用于显示末尾提示
- `status`、`created_at`、`updated_at`

基础地址只允许 HTTPS。网关默认拒绝解析到回环、私有、链路本地或组播地址的上游目标。

## 11. model_routes

- `provider_id`
- `public_name`，对外模型名，唯一且创建后不可修改
- `display_name`
- `upstream_name`
- `input_price_micros`、`output_price_micros`，人民币微元 / 百万 tokens
- `status`、`created_at`、`updated_at`

只有模型路由和提供商都启用时，`/v1/models` 才会返回该模型。

## 12. api_usages

- `user_id`、`api_key_id`、`model_route_id`
- `request_id`、`endpoint`
- `input_tokens`、`output_tokens`
- `reserved_micros`、`cost_micros`、`estimated`
- `upstream_request_id`、`created_at`、`finished_at`

该表保存调用审计和结算依据，不保存请求提示词、完整响应或上游凭据。

## 13. 数据库扩展

使用 Ent 生成数据访问层，以 MySQL 为默认数据库，同时避免在业务代码中散落只适用于
MySQL 的 SQL，让后续更换数据库时只需要替换驱动和迁移细节。
