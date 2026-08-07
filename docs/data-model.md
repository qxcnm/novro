# 数据模型

## 1. 关系概览

```text
User
├── BillingGroup
├── UserSession[]
├── UserIdentity[]
├── APIKey[]
├── Wallet
├── TopUpOrder[]
└── APIUsage[]

Provider
└── ModelRoute[]

BillingGroup
└── User[] / APIUsage[]

UpstreamModel
├── ModelRoute[]
└── APIUsage[]

Wallet
└── WalletEntry[]

PaymentConfig
└── administrator-managed payment provider settings
```

当前认证范围不增加组织、项目和项目成员关系。

## 2. users

建议字段：

- `id`
- `billing_group_id`，当前计费分组
- `username`，唯一
- `email`，规范化为小写且唯一；新本地账号和 OIDC 自动创建账号必须提供，历史账号迁移时可空
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

`issuer + subject` 唯一确定一个外部身份，`user_id + issuer` 也具有唯一约束。身份表邮箱
保留上游声明，OIDC 自动创建用户时还会写入规范化后的 `users.email`。系统不按邮箱自动
合并本地账号，OIDC access token、ID Token 和 Client Secret 不写入该表。

## 5. system_settings

当前仅保存 `initial_admin_created` 安装标记。该表不保存初始化令牌、密码、连接信息或
其他部署密钥。标记与第一个管理员在同一数据库事务中创建，保证初始化只能完成一次。

## 6. 当前迁移状态

`0001_users_and_sessions.sql` 创建用户和会话；
`0002_registration_oidc_and_setup.sql` 将本地密码改为可空，并创建外部身份与安装标记；
`0003_api_keys.sql` 创建用户 API Key；`0004_providers.sql` 创建加密提供商配置；
`0005_wallets_model_routes_and_usage.sql` 创建钱包、流水、模型路由和调用用量；
`0006_idempotent_wallet_entries.sql` 为调用预占和退款增加幂等唯一约束；
`0007_upstream_models_billing_groups_and_precise_usage.sql` 创建上游模型、计费分组，
并补齐缓存维度和计费快照字段；`0008_model_catalog_and_provider_routes.sql` 将模型目录
从提供商凭据配置中解耦，并保留路由到目录模型的关联；
`0009_seed_popular_model_catalog.sql` 归一化官方提供商目录名称，并按 2026-08-06 的官方
人民币价格初始化 DeepSeek V4、GLM-5.2/4.7 FlashX 和 Kimi K3/K2.7/K2.6；
`0010_epay_top_up_orders.sql` 创建充值订单，并为余额流水增加 `top_up` 类型；
`0011_payment_configs.sql` 创建管理员可编辑的支付配置表，商户密钥以密文保存；
`0012_soft_delete_admin_resources.sql` 为提供商、模型目录、模型路由和计费分组增加
`deleted_at`；`0013_user_email.sql` 为本地账号和 OIDC 账号增加统一的唯一邮箱字段，并回填
没有跨用户冲突的已有 OIDC 邮箱；`0014_flexible_payment_settings_and_top_up_credits.sql`
增加结构化支付方式、充值规则和订单实际到账金额。
服务启动时自动核对迁移历史并补齐未执行文件，仍保留显式 `migrate` 命令用于部署前检查。

提供商、模型目录、模型路由和计费分组使用软删除。普通查询和网关路由解析排除
`deleted_at` 非空记录，但历史用量、钱包流水及外键关联继续保留。删除提供商或目录模型时，
关联模型路由在同一事务中一并软删除；默认计费分组以及仍分配给用户的分组拒绝删除。

## 7. wallets

每个用户一个钱包，建议字段：

- `id`
- `user_id`，唯一
- `balance_micros`，定点整数，1 元人民币等于 1,000,000 微元；结算极端超出预占时允许短暂负数，等待管理员补充余额
- `created_at`
- `updated_at`

余额使用定点数类型，避免使用浮点数。钱包余额和流水变更需要在同一事务中完成。

## 8. wallet_entries

建议字段：

- `id`
- `wallet_id`
- `amount_micros`
- `balance_after_micros`
- `entry_type`，`manual_adjustment`、`top_up`、`usage_reservation`、`usage_refund` 或 `usage_settlement`
- `reference_id`
- `description`
- `actor_user_id`，人工调整时记录管理员
- `created_at`

余额能力保留流水表，方便审计、人工调整和调用结算。

调用开始时先写入负的 `usage_reservation`，成功后写入释放差额的 `usage_refund`；两者的
净变化就是最终调用费用。钱包行使用 `SELECT ... FOR UPDATE` 语义锁定。同一钱包的
`reference_id + entry_type` 唯一，使余额预占和退款可以安全重试；相同金额的重复请求
视为已完成，不同金额的重复请求视为冲突。若真实 usage 超过预占，会追加一条
`usage_settlement` 补扣流水，不能静默截断费用。

## 9. top_up_orders

每次在线充值创建一个订单，建议字段：

- `id`
- `user_id`
- `out_trade_no`，Novro 商户订单号，全局唯一
- `provider`，当前为 `epay`
- `channel`，支付网关渠道代码
- `amount_micros`，精确到分的正整数人民币微元
- `credited_micros`，支付成功后实际增加的钱包余额，包含适用的充值赠送
- `status`，`pending` 或 `paid`
- `provider_trade_no`，支付平台订单号，支付成功后唯一
- `paid_at`
- `created_at`
- `updated_at`

异步通知先锁定订单，再用 `amount_micros` 核对支付金额和渠道并锁定用户钱包；钱包使用
`credited_micros` 入账。订单状态、钱包余额和引用订单 ID 的 `top_up` 流水在同一事务内提交。
重复成功通知读取到 `paid` 后直接确认，不重复增加余额。

## 10. payment_configs

支付配置以 `provider=epay` 的单行记录保存，包含启用状态、接口地址、商户 ID、站点名称、
结构化支付方式、全局金额范围、预设金额和赠送档位。`channels` 继续作为旧配置兼容字段；
新请求从启用的支付方式推导渠道。商户密钥字段只保存使用
`NOVRO_PROVIDER_ENCRYPTION_SECRET` 加密的密文；管理员读取接口只返回
`has_merchant_key`，不会返回密钥。回调和返回地址由 `NOVRO_PUBLIC_URL` 生成，不作为可编辑
凭据保存。用户充值服务按请求读取这条记录，因此管理员保存后无需重启即可影响充值弹窗。

## 11. api_keys

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

## 11. providers

- `id`、`code`、`display_name`
- `protocol`，`openai` 或 `anthropic`
- `base_url`
- `encrypted_api_key`，AES-256-GCM 密文，仅服务端读取
- `api_key_hint`，只用于显示末尾提示
- `status`、`created_at`、`updated_at`

基础地址只允许 HTTPS。网关默认拒绝解析到回环、私有、链路本地或组播地址的上游目标。

## 12. model_routes

- `provider_id`
- `upstream_model_id`，指向独立模型目录记录
- `public_name`，对外模型名，唯一且创建后不可修改
- `display_name`
- 旧版 `upstream_name`、输入/输出价格字段保留用于迁移兼容，新的计费读取上游模型
- `status`、`created_at`、`updated_at`

只有模型路由、提供商配置和目录模型都启用时，`/v1/models` 才会返回该模型。

## 13. billing_groups

- `code`、`display_name`
- `multiplier_bps`，10000 表示 1.0000 倍
- `is_default`、`status`、`created_at`、`updated_at`

用户创建时默认进入启用的默认分组，管理员可在用户编辑抽屉中切换到其他启用分组。
默认分组不能停用。

## 14. upstream_models

- `provider_name`，普通文本，不绑定提供商凭据配置
- `upstream_name`、`display_name`
- `input_price_micros`、`output_price_micros`
- `cache_read_price_micros`、`cache_write_price_micros`、`cache_write_1h_price_micros`
- `request_price_micros`，按次固定费用
- `pricing_configured`，同步发现后为 `false`，完成价格维护后变为 `true`
- `status`、`created_at`、`updated_at`

所有单价是人民币微元 / 百万 tokens，固定费用是人民币微元 / 次。同步发现的新模型默认
停用且不能在未完成定价时启用，避免把未知成本按零价格结算。

初始化目录只包含官方价格可以由单一输入、缓存命中和输出单价准确表达的当前模型。
GLM-5、GLM-5.1 和 GLM-4.7 等按输入/输出长度分档计费的模型不写入固定价格，避免按
最低档少扣或按最高档多扣；支持阶梯价格前只能保持待定价状态。

## 15. api_usages

- `user_id`、`api_key_id`、`model_route_id`
- `request_id`、`endpoint`
- `input_tokens`、`uncached_input_tokens`、`cache_read_input_tokens`、`cache_write_input_tokens`、
  `cache_write_1h_input_tokens`、`output_tokens`
- 当时采用的各类价格、`base_cost_micros`、`multiplier_bps`、计费分组和算法版本
- `reserved_micros`、`cost_micros`、`estimated`
- `upstream_request_id`、`created_at`、`finished_at`

网关按各维度 `token × 单价` 汇总原始分子，乘计费分组倍率后只向上取整一次；
该表保存调用审计和不可变结算依据，不保存请求提示词、完整响应或上游凭据。

## 16. 数据库扩展

使用 Ent 生成数据访问层，以 MySQL 为默认数据库，同时避免在业务代码中散落只适用于
MySQL 的 SQL，让后续更换数据库时只需要替换驱动和迁移细节。
