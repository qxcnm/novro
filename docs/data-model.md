# 数据模型

## 1. 关系概览

```text
User
├── UserSession[]
├── UserIdentity[]
├── APIKey[]
├── Wallet
├── TopUpOrder[]
└── APIUsage[]

APIKey
├── BillingGroup
└── GatewayOperation[]

Provider
├── BillingGroup
└── ModelRoute[]

BillingGroup
└── APIKey[] / Provider[] / APIUsage[]

UpstreamModel
├── ModelRoute[]
└── APIUsage[]

Wallet
└── WalletEntry[]

PaymentConfig
└── administrator-managed payment provider settings

EmailVerificationCode
└── standalone pre-registration email challenge
```

当前认证范围不增加组织、项目和项目成员关系。

## 2. users

建议字段：

- `id`
- `invite_code`，创建时生成的唯一邀请码，之后不可修改
- `referred_by_user_id`，可空且创建后不可修改，指向邀请人
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

保存 `initial_admin_created` 安装标记和不含秘密的应用设置。`referral_reward_bps` 记录管理员
设置的全局邀请返现基点；`gateway_request_settings` 以 JSON 原子保存 SSE 心跳开关、心跳间隔、
上游总超时、流式空闲超时，以及输入/输出预占 Token 上限。旧 JSON 缺少预占上限时分别补
默认值 16384 和 1024。该表不保存初始化令牌、密码、连接信息或其他部署密钥。初始化标记
与第一个管理员在同一数据库事务中创建，保证初始化只能完成一次。

## 6. 当前迁移状态

当前迁移从 `0001_initial_schema.sql` 开始，并通过后续有序迁移演进。`0009_gateway_billing_safety.sql`
新增持久化网关 operation 和计费补偿流水类型；部署包含本功能的代码前必须先成功执行到 `0009`。
迁移创建认证、用户、API Key、供应商、模型目录、模型路由、计费分组、钱包、充值、
邮件、邀请返现、用量审计和结算恢复相关表，并写入默认计费分组与默认邀请返现比例。
服务启动不修改数据库结构；部署和本地升级都必须显式运行 `migrate`。旧库升级不再作为
当前迁移文件的目标，保留数据升级时需要使用对应历史版本先完成迁移。

提供商、模型目录、模型路由和计费分组使用软删除。普通查询和网关路由解析排除
`deleted_at` 非空记录，但历史用量、钱包流水及外键关联继续保留。删除提供商或目录模型时，
关联模型路由在同一事务中一并软删除；默认计费分组以及仍被启用 API Key 或未删除供应商
使用的分组拒绝删除。

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
- `entry_type`，`manual_adjustment`、`top_up`、`referral_reward`、`usage_reservation`、`usage_refund`、`usage_settlement` 或 `usage_compensation`
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

`usage_compensation` 只用于经核对的旧版 `token-v2`、成功状态、`estimated=true` 异常用量。
它使用原调用 `request_id` 作为引用并全额冲回该记录费用；同一钱包、请求和流水类型的唯一约束
确保并发或重复执行只补偿一次。生产补偿必须基于审核后的 request ID 清单执行，不能按模糊条件
直接批量修改余额。

`referral_reward` 使用被邀请用户的充值订单 ID 作为 `reference_id`。同一钱包、订单和流水
类型的唯一约束保证重复支付回调只能写入一次返现；返现与充值入账在同一事务内提交。

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
- `billing_group_id`，Key 绑定的计费分组
- `name`
- `key_prefix`
- `key_hash`
- `status`，`active` 或 `revoked`
- `last_used_at`
- `created_at`
- `revoked_at`

完整 Key 只在创建成功时显示一次。数据库只保存哈希和前缀。Key 前缀用于用户识别自己的 Key，但不能用于认证。
认证时会同时校验用户、Key 和 Key 所属计费分组均启用；停用分组后，该组 Key 不再通过网关认证。

## 11. providers

- `id`、`billing_group_id`、`code`、`display_name`
- `protocol`，`openai` 或 `anthropic`
- `base_url`
- `model_list_path`，可选的模型获取路径覆盖值
- `weight`，1 到 1000000 的请求优先级，默认 100，数值越大越优先
- `encrypted_api_key`，AES-256-GCM 密文，仅服务端读取
- `api_key_hint`，只用于显示末尾提示
- `status`、`created_at`、`updated_at`

`model_list_path` 保存可选的模型获取路径；空值保留协议默认路径，非空值必须是以 `/` 开头的
站点绝对路径，并在模型目录同步时覆盖默认拼接规则。基础地址允许 HTTP 或 HTTPS，以支持自建和第三方网关；生产环境建议使用 HTTPS，避免 API Key
明文传输。网关默认拒绝解析到回环、私有、链路本地、未指定或组播地址的上游目标，并禁止跟随
上游重定向。
新增和修改供应商时只能选择启用中的计费分组。网关解析候选路由时只会使用与当前 API Key
同一计费分组的供应商，并按 `weight` 降序请求；同权重保留稳定路由顺序。

## 12. model_routes

- `provider_id`
- `upstream_model_id`，指向独立模型目录记录
- `public_name`，对外模型名，创建后不可修改；自动关联时使用全局目录模型 ID，多个提供商因此进入同一个故障切换池
- `display_name`
- 旧版 `upstream_name`、输入/输出价格字段保留用于迁移兼容，新的计费读取全局模型目录
- `status`、`created_at`、`updated_at`

`public_name + provider_id + upstream_model_id` 组合唯一，防止完全相同的渠道重复配置。同一
`public_name` 下只有模型路由、提供商配置、提供商计费分组和目录模型都启用，且提供商分组
与当前 API Key 分组一致的记录才进入候选池；
`/v1/models` 对同名候选去重后返回。

## 13. billing_groups

- `code`、`display_name`
- `multiplier_bps`，10000 表示 1.0000 倍
- `is_default`、`status`、`created_at`、`updated_at`

计费分组用于 API Key 和供应商两个维度。`is_hidden` 标记分组是否只对管理员和已授权用户可见，
用户的 `can_access_hidden_groups` 控制普通成员是否拥有该权限。创建 API Key 时用户选择分组；新增或修改供应商时
管理员选择分组。调用时以 API Key 所属分组作为结算倍率，并只访问同分组供应商。默认分组
用于空库初始化和默认选择，不能停用。

## 14. upstream_models

- `provider_name`，普通厂商标签，不绑定提供商凭据配置，也不决定价格归属
- `upstream_name`，全局唯一的精确模型 ID；`display_name` 为管理界面显示名称
- `input_price_micros`、`output_price_micros`
- `cache_read_price_micros`、`cache_write_price_micros`、`cache_write_1h_price_micros`
- `request_price_micros`，按次固定费用
- `pricing_configured`，新同步模型默认为 `false`；只有管理员在目录定价页保存价格后才会设为 `true`
- `status`、`created_at`、`updated_at`

所有单价是人民币微元 / 百万 tokens，固定费用是人民币微元 / 次。同一模型 ID 无论被多少
提供商暴露都只维护这一套价格。同步只发现精确 ID，不读取或导入上游返回的 `pricing` 字段；管理员
必须在目录定价页维护价格，未知模型默认停用且不能在未完成定价时启用，避免把未知成本按零价格结算
或复制价格卡。自动关联路由的对外名称
与精确上游 ID 一致，历史的提供商前缀和数字后缀由后续迁移修复。

初始化迁移不再内置官方模型目录。管理员通过供应商同步发现模型后，在模型目录维护价格并启用。
GLM-5、GLM-5.1 和 GLM-4.7 等按输入/输出长度分档计费的模型不应写入固定价格，避免按
最低档少扣或按最高档多扣；支持阶梯价格前只能保持待定价状态。

## 15. api_usages

- `user_id`、`api_key_id`、`model_route_id`
- `request_id`、`endpoint`
- `status_code`、`error_code`、`error_message`、`duration_ms`
- `input_tokens`、`uncached_input_tokens`、`cache_read_input_tokens`、`cache_write_input_tokens`、
  `cache_write_1h_input_tokens`、`output_tokens`
- 当时采用的各类价格、`base_cost_micros`、`multiplier_bps`、计费分组和算法版本
- `reserved_micros`、`cost_micros`、`estimated`
- `upstream_request_id`、`created_at`、`finished_at`

网关按各维度 `token × 单价` 汇总原始分子，乘计费分组倍率后只向上取整一次；所有候选上游均失败的请求也会写入
该表，但费用和 Token 保持为 0，并通过状态码和错误字段标记失败。该表保存调用审计和不可变结算依据，
不保存请求提示词、完整响应或上游凭据。`token-v3-confirmed-usage` 算法只结算上游明确报告的
Token 维度；缺失维度保持 0 并设置 `estimated`，保守请求预占只用于余额校验，不能作为最终费用。
预占和最终费用都应用 API Key 请求时绑定的 `multiplier_bps`。

## 16. gateway_operations

- `id`，也是对外 `request_id`
- `user_id`、`api_key_id`
- `idempotency_key_hash`，只保存客户端幂等键的 SHA-256，不保存原值
- `request_hash`，绑定 endpoint 与原始请求体
- `endpoint`
- `status`，`processing`、`pending_settlement`、`pending_unknown`、`completed` 或 `failed`
- `reserved_micros`
- `settlement_json`，完整且可验证的最终 `UsageInput` 快照
- `failure_code`、`created_at`、`updated_at`

`api_key_id + idempotency_key_hash` 唯一。operation 创建、钱包行锁、预占扣减和
`usage_reservation` 流水在同一事务中完成，因此同键并发请求只能有一个创建者。相同键和相同
请求读取已有 operation，不再次调用上游；相同键绑定不同请求时返回冲突。

成功终态先把不可变 usage 快照写为 `pending_settlement`，再在钱包事务中验证 operation、用户、
API Key、endpoint、预占金额和对应预占流水，最后写 `api_usages`、退款或补扣流水。后台周期恢复
`pending_settlement`。`pending_unknown` 表示请求可能已送达上游但没有可安全结算的终态；系统
保留预占且不自动重放或退款，必须根据提供商请求记录和 `request_id` 人工核对。长期
`processing` 也需要运维核查，不能仅凭超时自动释放。

## 17. 数据库扩展

使用 Ent 生成数据访问层，以 MySQL 为默认数据库，同时避免在业务代码中散落只适用于
MySQL 的 SQL，让后续更换数据库时只需要替换驱动和迁移细节。
