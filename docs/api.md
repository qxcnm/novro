# API 约定

## 当前控制台 API

本阶段已实现以下同源 Cookie 会话接口：

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| `POST` | `/api/auth/login` | 公开 | 使用用户名或邮箱登录并设置 HttpOnly 会话 Cookie |
| `GET` | `/api/auth/options` | 公开 | 返回可公开的注册、初始化和 OIDC 开关 |
| `POST` | `/api/auth/setup` | 公开 + 安装令牌 | 仅在空库中创建第一个管理员 |
| `POST` | `/api/auth/register` | 公开 + 配置开关 | 注册普通成员并建立会话 |
| `POST` | `/api/auth/register/send-code` | 公开 + 配置开关 | 先检查邮箱是否已注册；未注册时才发送一次性验证码，已注册返回 `email_taken` |
| `GET` | `/api/admin/email` | 管理员 | 读取不含 SMTP 密码的邮件配置 |
| `PUT` | `/api/admin/email` | 管理员 | 保存并立即应用 SMTP 配置；空密码保留原凭据 |
| `POST` | `/api/admin/email/test` | 管理员 | 使用已保存配置发送测试邮件 |
| `GET` | `/api/auth/oidc/start` | 公开 | 启动 OIDC Authorization Code + PKCE 流程 |
| `GET` | `/api/auth/oidc/callback` | 公开 | 验证 OIDC 回调并建立 Novro 会话 |
| `POST` | `/api/auth/logout` | 登录可选 | 撤销当前会话并清除 Cookie |
| `GET` | `/api/auth/me` | 登录 | 返回当前用户 |
| `PATCH` | `/api/account/profile` | 登录 | 修改当前用户显示名称 |
| `GET` | `/api/account/referral` | 登录 | 当前用户的邀请码、返现汇总、邀请链接，以及最近 20 条返现和邀请记录 |
| `GET` | `/api/admin/referral` | 管理员 | 读取当前全局邀请返现比例 |
| `PUT` | `/api/admin/referral` | 管理员 | 保存全局邀请返现比例并立即用于新的充值到账 |
| `GET` | `/api/admin/gateway-settings` | 管理员 | 读取 SSE 心跳和上游超时设置 |
| `PUT` | `/api/admin/gateway-settings` | 管理员 | 保存请求生命周期设置并立即用于新的模型请求 |
| `GET` | `/api/account/announcement` | 登录 | 读取当前启用的系统公告；没有公告时返回 `available: false` |
| `GET` | `/api/admin/announcement` | 管理员 | 读取系统公告内容、启用状态和最近更新时间 |
| `PUT` | `/api/admin/announcement` | 管理员 | 保存纯文本系统公告及启用状态 |
| `GET` | `/api/admin/users` | 管理员 | 分页、搜索和状态筛选 |
| `POST` | `/api/admin/users` | 管理员 | 创建管理员或成员 |
| `PATCH` | `/api/admin/users/{id}` | 管理员 | 修改显示名称或角色 |
| `PATCH` | `/api/admin/users/{id}/status` | 管理员 | 启用或停用用户 |
| `POST` | `/api/admin/users/{id}/reset-password` | 管理员 | 重置密码并撤销旧会话 |
| `GET` | `/api/account/api-keys` | 登录 | 当前用户的 Key 元数据 |
| `GET` | `/api/account/billing-groups` | 登录 | 当前用户有权选择的启用计费分组；隐藏分组仅对管理员和该分组已授权用户返回 |
| `POST` | `/api/account/api-keys` | 登录 | 指定计费分组创建 Key，完整值只返回一次 |
| `DELETE` | `/api/account/api-keys/{id}` | 登录 | 撤销当前用户的 Key |
| `GET` | `/api/account/models` | 登录 | 当前可用模型和指定或默认分组的结算单价 |
| `GET` | `/api/account/balance` | 登录 | 当前余额和最近流水 |
| `GET` | `/api/account/usage` | 登录 | 当前用户最近调用用量 |
| `GET` | `/api/account/usage/rate` | 登录 | 当前用户最近 60 秒的 RPM 与 TPM |
| `GET` | `/api/account/top-ups/config` | 登录 | 充值开关、支付方式、金额范围、预设金额和赠送档位 |
| `GET` | `/api/account/top-ups` | 登录 | 当前用户最近充值订单 |
| `POST` | `/api/account/top-ups` | 登录 | 创建充值订单和服务端签名支付表单 |
| `POST` | `/api/account/top-ups/{out_trade_no}/reconcile` | 登录 | 只读查询本人原订单的支付结果，平台确认已支付后幂等入账 |
| `GET/POST` | `/api/payments/epay/notify` | 易支付签名 | 验签并幂等完成异步充值入账 |
| `GET` | `/api/payments/epay/return` | 易支付签名 | 验签同步返回结果、幂等入账并跳回余额页 |
| `GET` | `/api/admin/payments` | 管理员 | 查看易支付安全配置摘要，不返回商户密钥 |
| `PUT` | `/api/admin/payments` | 管理员 | 保存易支付商户配置、支付方式和充值规则 |
| `GET` | `/api/admin/top-ups` | 管理员 | 分页查询全部用户的充值订单 |
| `GET` | `/api/admin/api-keys` | 管理员 | 分页审计所有用户的 Key 元数据 |
| `POST` | `/api/admin/api-keys/{id}/revoke` | 管理员 | 撤销任意用户的 Key |
| `GET` | `/api/admin/users/{id}/balance` | 管理员 | 查看用户余额和流水 |
| `POST` | `/api/admin/users/{id}/balance-adjustments` | 管理员 | 原子调整余额并写入流水 |
| `GET` | `/api/admin/providers` | 管理员 | 搜索和筛选提供商 |
| `POST` | `/api/admin/providers` | 管理员 | 指定计费分组创建加密凭据配置 |
| `PATCH` | `/api/admin/providers/{id}` | 管理员 | 修改提供商配置、计费分组或替换凭据 |
| `PATCH` | `/api/admin/providers/{id}/status` | 管理员 | 启用或停用提供商 |
| `DELETE` | `/api/admin/providers/{id}` | 管理员 | 软删除提供商及其模型路由 |
| `POST` | `/api/admin/providers/{id}/models/sync` | 管理员 | 从提供商发现模型 ID；全局目录已有 ID 复用原价格，新 ID 建立待定价记录 |
| `POST` | `/api/admin/providers/{id}/models` | 管理员 | 从模型目录批量创建关联路由 |
| `GET` | `/api/admin/upstream-models` | 管理员 | 搜索和筛选模型目录 |
| `POST` | `/api/admin/upstream-models` | 管理员 | 创建全局唯一模型目录记录和完整价格维度 |
| `PATCH` | `/api/admin/upstream-models/{id}` | 管理员 | 修改全局模型目录和价格维度 |
| `PATCH` | `/api/admin/upstream-models/{id}/status` | 管理员 | 启用或停用目录模型 |
| `DELETE` | `/api/admin/upstream-models/{id}` | 管理员 | 软删除目录模型及其模型路由 |
| `GET` | `/api/admin/model-routes` | 管理员 | 搜索和筛选模型路由 |
| `POST` | `/api/admin/model-routes` | 管理员 | 创建对外模型到上游模型的映射 |
| `PATCH` | `/api/admin/model-routes/{id}` | 管理员 | 修改上游模型映射和显示名称 |
| `PATCH` | `/api/admin/model-routes/{id}/status` | 管理员 | 启用或停用模型路由 |
| `DELETE` | `/api/admin/model-routes/{id}` | 管理员 | 软删除模型路由 |
| `GET` | `/api/admin/billing-groups` | 管理员 | 查看计费分组、隐藏属性、授权用户、API Key 数和供应商数 |
| `POST` | `/api/admin/billing-groups` | 管理员 | 创建计费分组、倍率和隐藏组授权用户 |
| `PATCH` | `/api/admin/billing-groups/{id}` | 管理员 | 修改分组名称、倍率、隐藏属性或授权用户 |
| `PATCH` | `/api/admin/billing-groups/{id}/status` | 管理员 | 启用或停用非默认分组 |
| `DELETE` | `/api/admin/billing-groups/{id}` | 管理员 | 软删除未使用的非默认分组 |

控制台 `/api/*` 的写请求（除 `GET`、`HEAD` 和 `OPTIONS` 外）必须携带
`Origin`，并且必须匹配 `NOVRO_ALLOWED_ORIGINS` 中配置的完整来源；缺失或不匹配时
返回 `403 invalid_origin`。易支付异步通知路径是唯一例外，它不依赖浏览器来源，而是
校验商户 ID 和通知签名。反向代理必须保留浏览器的 `Origin` 请求头。模型兼容
`/v1/*` 使用 API Key 的机器请求不受这条浏览器来源规则影响。错误使用 `error.code`
与安全的中文提示，不返回 SQL、连接字符串、密码哈希或会话令牌。

隐藏计费分组的创建请求可提交 `authorized_user_ids` UUID 数组；修改请求可用同名字段整体替换
授权名单。名单只能包含普通成员，不能包含管理员或未知用户；公开分组不能携带授权名单。
管理员自动拥有全部隐藏组权限。管理列表中的每个分组返回 `authorized_users` 摘要，供控制台回显。

每个 HTTP 请求都会返回 `X-Novro-Request-ID`。错误 JSON 还会返回顶层
`request_id`，控制台 API、网关鉴权失败和网关上游错误都使用同一 ID，便于结合结构化
日志排查。客户端提供的同名请求头不会被采纳。

修改用户角色和停用用户都受最后启用管理员保护。系统会在数据库事务内锁定启用的
管理员集合，不能通过角色降级或状态变更移除最后一个可用管理员。

本地注册、首次管理员初始化和管理员创建用户都要求 `username`、`email` 与 `password`；密码必须为 8 到 72 字节，并同时包含至少一个英文字符和一个数字。公开注册还需先调用 `/api/auth/register/send-code`，该请求会先检查邮箱是否已注册，已注册时返回 `email_taken`，未注册才发送验证码，再在注册请求中提交 `verification_code`。验证码有效期 10 分钟、单次使用，同一邮箱 60 秒内只能发送一次。通过邀请链接注册时可额外提交 `referral_code`；邀请码只在创建账号时绑定，注册后不能更换邀请人。
邮箱会转为小写并保持唯一。登录请求继续使用兼容字段 `username`，该字段可以填写用户名
或邮箱。登录、注册和初始化响应设置 Cookie 后，控制台会再调用 `/api/auth/me` 确认会话；
只有明确的 `401` 会被视为退出登录，临时网络错误或服务端错误不会清除前端登录状态。

OIDC 使用 Issuer Discovery、Authorization Code、PKCE、state 和 nonce。外部身份以
`issuer + subject` 唯一绑定。OIDC 自动创建的用户会将规范化邮箱写入与本地账号相同的
`users.email` 字段，但不会仅根据邮箱自动合并已有本地账号。上游 access token、ID Token
和 Client Secret 不发送给浏览器。

`/v1` 模型兼容 API 已实现基础非流式和 SSE 流式转发。上游请求会使用管理员配置的
提供商凭据，浏览器和 API 客户端永远不会收到上游密钥。面向 API 使用者的完整示例由
前端 `/docs` 提供；公开模型目录的官方牌价不是当前 Novro 结算价，实际结算单价以管理员
模型目录配置为准。上游请求固定使用 HTTP/1.1，以兼容会宣传 HTTP/2 但在 TLS 或请求阶段
提前断开的 OpenAI 兼容网关。上游返回重定向时网关不会自动跟随到新地址，也不会把当前
POST 重放到其他渠道，避免把提供商凭据带到未预期的主机或重复执行模型调用；该请求按明确
失败处理并释放预占。
上游连接、TLS 握手和等待响应头分别有 15 秒、30 秒和 180 秒固定上限。管理员还可以在
`/admin/gateway` 配置覆盖连接、响应头、响应体和路由重试的上游总超时，以及只针对流式响应的
上游空闲超时；两项设为 `0` 时关闭。SSE 心跳默认开启并每 15000 毫秒在完整事件边界发送
`novro-keepalive` 注释，心跳不会计作上游活动，也不会改变模型事件内容。

## 1. 基础信息

默认 API 前缀：

```text
/v1
```

客户端使用 API Key 认证：

```http
Authorization: Bearer nvr_xxxxxxxxxxxxxxxxx
Content-Type: application/json
```

API Key 属于单个用户，并绑定一个计费分组。数据库只保存 SHA-256 哈希和前缀。第一版不引入
项目 Key、组织 Key 或临时 Key。标准入口使用 `Authorization: Bearer`；Anthropic 客户端也可
使用 `x-api-key`。认证会同时校验用户、Key 和 Key 所属计费分组均启用。

## 2. 模型列表

```http
GET /v1/models
```

兼容别名：`GET /v1/model`。两个路径返回相同结果。

返回当前 API Key 所属计费分组可以使用的启用模型。模型名称是管理员配置的 `public_name`；
自动关联时它等于全局目录的精确 `upstream_name`，并按目录的 `provider_name` 返回厂商标签。
同步只是发现上游模型，只有管理员在目录定价并选择关联后才会出现在这里。同步聚合渠道时不会
把渠道代码写入公开模型名，管理员手工别名保持不变。同名的多条启用路由只返回一个模型项，
响应不会暴露上游 API Key 或加密配置。

同一 `public_name`、同一 API 协议下的启用路由组成渠道池。网关按照提供商的 `weight` 从高到低请求，
权重相同时保持稳定的路由顺序。只有 DNS、拨号或 TLS 等发生在取得连接前的明确连接失败，才会
在当前渠道有限重试并继续下一候选渠道。一旦取得连接、收到任意 HTTP 响应，或无法证明请求未被
上游接收，就不会自动重放 POST。明确的非 `2xx`、重定向或失败事件直接结束本次 operation；已取得
连接后的网络错误、`2xx` 响应体读取失败以及没有明确终态的流进入 `pending_unknown`，保留预占供
人工核对。普通响应只有在完整读取且结算意图已持久化后才写给客户端；SSE 不会拼接不同渠道的流。

## 3. OpenAI Chat Completions

```http
POST /v1/chat/completions
```

兼容常见 OpenAI Chat Completions 请求，并把 `model` 映射成上游名称。当前覆盖：

- 非流式 JSON 响应
- `stream: true` 的 SSE 流式响应
- `messages`
- `model`
- `temperature`
- `max_tokens`
- `tools`
- `tool_choice`
- `tool_calls`

## 4. OpenAI Responses

```http
POST /v1/responses
```

兼容 OpenAI Responses 风格请求，转发文本输入、模型选择、流式输出和工具调用字段。请求
会复用同一套 API Key 鉴权、余额预占、usage 结算和模型路由逻辑；字段最终是否可用仍取决于上游模型。

## 5. Anthropic Messages

```http
POST /v1/messages
```

兼容 Anthropic Messages 风格请求，并仅路由到协议为 Anthropic 的提供商：

- `model`
- `messages`
- `system`
- `max_tokens`
- `temperature`
- `stream`
- `tools`
- 工具调用结果

产品文档和接口实现统一使用 Anthropic 官方复数路径 `/v1/messages`。不将 `/v1/message` 作为主路径。

## 6. 余额规则

- 请求进入网关后先验证用户、Key、模型、提供商和账户状态。
- 按 Novro 全局模型目录配置的普通输入、缓存命中、缓存创建（5 分钟/1 小时）、输出和按次价格，
  以人民币微元（1 元 = 1,000,000 微元）结算；API Key 所属计费分组倍率作用于汇总费用。
- 网关用 `ceil(JSON 请求字节数 / 4) + 64` 估算输入，输入预占默认上限为 16384 Token；
  输出按请求声明的最大值估算，默认预占上限为 1024 Token。两个上限由 `/admin/gateway`
  配置，只限制余额冻结，不截断发送给模型的上下文或输出。网关按候选渠道中的最高可能费用
  做一次预占；成功后按实际命中渠道的上游 usage 和单价结算并释放差额。候选渠道只来自与当前 API Key
  同一计费分组的供应商。用户可用模型页按所选分组、同协议渠道池的各维度最高单价展示，
  避免实际结算高于页面价格。
- 供应商调用成功后，只根据上游明确返回的 usage 扣减用户余额。usage 缺少输入或输出维度时，
  缺失维度按 0 结算并释放对应预占，不使用请求字节数或最大输出 token 猜测最终费用。
- 余额不足时拒绝请求，不向供应商发起调用。
- 实际费用低于预占时释放差额；实际费用高于预占时写入唯一的 `usage_settlement` 流水补扣，
  不会把费用截断为预占。余额可能因此短暂为负数，但同一请求不会重复补扣。
- 流式 usage 按上游的完整累计快照更新。`response.completed`、`response.incomplete`、`[DONE]`
  或 Anthropic `message_stop` 是可结算终态；明确 failed、cancelled 或 error 不收费并释放预占。
  流没有明确终态就结束时进入 `pending_unknown`，不会用中途观察到的片段自动结算或退款。
- 所有候选渠道均失败时仍会写入一条使用日志，记录最终网关状态码、耗时、错误码和安全的错误摘要；
  失败请求不会计入费用，已预占余额会按原流程释放。
- SSE 单行事件会分段透传并完整解析 usage，不设置固定的单行字节上限。
- 每个请求先在同一事务内创建 `gateway_operations` 记录、冻结余额并写预占流水。状态为
  `processing`、`pending_settlement`、`pending_unknown`、`completed` 或 `failed`；最终 usage
  先持久化为 `pending_settlement`，再完成钱包结算。后台会恢复待结算记录，数据库短暂故障或
  进程重启不会把已成功调用自动退款。
- 模型请求支持 `Idempotency-Key`。同一 API Key、同一键和相同请求体只创建一个 operation，
  并发或网络重试不会再次预占或调用上游；同一键用于不同请求体时返回 `409 idempotency_conflict`。
  上游只接收由 Novro 请求 ID 派生的幂等键，不会收到客户端原始键。
- 每次余额变化都写入余额流水，不能只更新余额总数。
- 钱包行在事务中锁定，避免并发请求共同花费同一份余额。
- `GET /api/account/usage` 支持 `offset`、`limit`、`api_key_id`、`model`、`status`、`search`
  和 `from`；返回匹配历史的总条数、Token 与费用汇总。`GET /api/account/balance` 支持
  `offset` 和 `limit`，并返回流水总数和当前尚未结算的调用预占。
- `GET /api/account/usage/rate` 使用最近 60 秒滚动窗口统计已写入的调用记录。RPM 包含成功和失败
  请求；TPM 为上游已确认的输入与输出 Token 之和，usage 不完整的请求只计入已确认部分。

### 在线充值

- 充值金额使用与钱包相同的人民币微元存储，只接受精确到分的金额；系统下限为 `¥1.00`，管理员可以在系统上限内调高全局范围以及每个支付方式的最低金额。
- 支付方式包含显示名称、易支付 `type` 处理标识、图标、启用状态和最低充值金额；用户端只接收已启用方式。
- 管理员可配置预设充值金额和按门槛递增的赠送比例。订单分别保存用户支付金额和实际到账金额；易支付签名与回调始终核对支付金额，赠送只影响入账余额。
- 创建订单后返回易支付 `submit.php` 的签名表单字段，商户密钥不会发送给浏览器。
- 仅 `TRADE_SUCCESS` 通知会进入入账流程；商户 ID、MD5 签名、订单号、渠道和金额必须全部匹配。
- 充值订单、钱包余额、`top_up` 流水在同一数据库事务内更新。支付平台重复通知同一订单时只入账一次。
- 服务端异步通知是主要到账路径。浏览器 `return_url` 也会把同一组易支付签名字段交给后端复核，在本地回调不可达或通知稍晚时作为补偿路径；未通过签名、金额、渠道和订单状态校验的返回结果不会入账。
- 管理员充值记录支持按订单号、网关流水号、用户、状态和支付方式筛选，不提供修改历史订单的入口。

### 邀请返现

- 每个用户创建时获得一个不可修改的唯一邀请码，邀请链接由 `NOVRO_PUBLIC_URL` 和该邀请码生成。
- 返现比例以基点保存在 `system_settings.referral_reward_bps`，管理员可在 `/admin/referral` 调整；`1000` 表示 10%，`0` 停止产生新的返现。数据库尚无该记录时才回退到 `NOVRO_REFERRAL_REWARD_BPS`。
- 被邀请用户的充值确认后，按实际支付金额计算返现，不把充值赠送金额计入返现基数。
- 充值完成事务会读取数据库中的当前比例；充值入账、邀请人返现、双方钱包余额和对应流水在同一数据库事务内完成，重复支付通知不会重复返现。比例调整只影响之后到账的充值，不重算历史奖励。
- “待确认”按被邀请用户尚未支付完成的充值订单估算，“总收入”只统计已写入邀请人钱包的 `referral_reward` 流水。
- 个人资料中的详情抽屉展示最近 20 条返现和邀请记录。邀请成员只返回显示名称、用户名和加入时间；返现记录额外返回对应充值金额、返现金额和到账时间，不返回邮箱、钱包 ID 或支付订单号。

### 用量字段兼容

网关读取 OpenAI/GLM 的 `prompt_tokens_details.cached_tokens`、DeepSeek 的
`prompt_cache_hit_tokens`/`prompt_cache_miss_tokens`、Kimi 的 `cached_tokens`，以及
Anthropic 的 `cache_read_input_tokens`、`cache_creation_input_tokens` 和 5 分钟/1 小时
明细。普通输入、缓存命中、缓存创建和输出分别乘各自单价，原始 Token 数、单价、分组
倍率、基础成本、最终费用和算法版本都会写入 `api_usages`，后续改价不会改变历史账单。

## 7. 错误响应

错误响应统一返回 JSON，并保留 OpenAI 风格的 `error` 对象。错误中不得泄露供应商密钥、
内部 URL 或数据库信息。错误响应包含顶层 `request_id`。单个渠道的错误不会直接暴露；所有
兼容渠道均失败时返回 `502 upstream_unavailable`，同时附带最后一次失败的安全错误码和原因，
便于区分上游 HTTP 错误、连接超时、客户端取消或上游地址自指等情况。上游地址不能配置为
当前网关对外提供服务的域名，否则请求会递归回到网关自身。

## 8. Kimi / Moonshot 配置约定

Kimi 作为 OpenAI 兼容提供商接入，推荐基础地址为 `https://api.moonshot.cn/v1`。在
管理员控制台保存提供商凭据后，系统会先尝试同步模型；管理员再从模型目录多选模型，
将对外模型名映射到账户实际可用的 Kimi 模型 ID。凭据始终只在服务端使用；调用日志会记录模型、API Key 名称、状态码、
耗时、Tokens、费用和是否为估算值，失败请求额外记录错误码和安全摘要，不记录请求体、Authorization 头或上游密钥。
