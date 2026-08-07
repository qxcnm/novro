# API 约定

## 当前控制台 API

本阶段已实现以下同源 Cookie 会话接口：

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| `POST` | `/api/auth/login` | 公开 | 使用用户名或邮箱登录并设置 HttpOnly 会话 Cookie |
| `GET` | `/api/auth/options` | 公开 | 返回可公开的注册、初始化和 OIDC 开关 |
| `POST` | `/api/auth/setup` | 公开 + 安装令牌 | 仅在空库中创建第一个管理员 |
| `POST` | `/api/auth/register` | 公开 + 配置开关 | 注册普通成员并建立会话 |
| `GET` | `/api/auth/oidc/start` | 公开 | 启动 OIDC Authorization Code + PKCE 流程 |
| `GET` | `/api/auth/oidc/callback` | 公开 | 验证 OIDC 回调并建立 Novro 会话 |
| `POST` | `/api/auth/logout` | 登录可选 | 撤销当前会话并清除 Cookie |
| `GET` | `/api/auth/me` | 登录 | 返回当前用户 |
| `PATCH` | `/api/account/profile` | 登录 | 修改当前用户显示名称 |
| `GET` | `/api/admin/users` | 管理员 | 分页、搜索和状态筛选 |
| `POST` | `/api/admin/users` | 管理员 | 创建管理员或成员 |
| `PATCH` | `/api/admin/users/{id}` | 管理员 | 修改显示名称、角色或计费分组 |
| `PATCH` | `/api/admin/users/{id}/status` | 管理员 | 启用或停用用户 |
| `POST` | `/api/admin/users/{id}/reset-password` | 管理员 | 重置密码并撤销旧会话 |
| `GET` | `/api/account/api-keys` | 登录 | 当前用户的 Key 元数据 |
| `POST` | `/api/account/api-keys` | 登录 | 创建 Key，完整值只返回一次 |
| `DELETE` | `/api/account/api-keys/{id}` | 登录 | 撤销当前用户的 Key |
| `GET` | `/api/account/balance` | 登录 | 当前余额和最近流水 |
| `GET` | `/api/account/usage` | 登录 | 当前用户最近调用用量 |
| `GET` | `/api/account/top-ups/config` | 登录 | 充值开关、支付方式、金额范围、预设金额和赠送档位 |
| `GET` | `/api/account/top-ups` | 登录 | 当前用户最近充值订单 |
| `POST` | `/api/account/top-ups` | 登录 | 创建充值订单和服务端签名支付表单 |
| `GET/POST` | `/api/payments/epay/notify` | 易支付签名 | 验签并幂等完成充值入账 |
| `GET` | `/api/admin/payments` | 管理员 | 查看易支付安全配置摘要，不返回商户密钥 |
| `PUT` | `/api/admin/payments` | 管理员 | 保存易支付商户配置、支付方式和充值规则 |
| `GET` | `/api/admin/top-ups` | 管理员 | 分页查询全部用户的充值订单 |
| `GET` | `/api/admin/api-keys` | 管理员 | 分页审计所有用户的 Key 元数据 |
| `POST` | `/api/admin/api-keys/{id}/revoke` | 管理员 | 撤销任意用户的 Key |
| `GET` | `/api/admin/users/{id}/balance` | 管理员 | 查看用户余额和流水 |
| `POST` | `/api/admin/users/{id}/balance-adjustments` | 管理员 | 原子调整余额并写入流水 |
| `GET` | `/api/admin/providers` | 管理员 | 搜索和筛选提供商 |
| `POST` | `/api/admin/providers` | 管理员 | 创建加密凭据配置 |
| `PATCH` | `/api/admin/providers/{id}` | 管理员 | 修改提供商配置或替换凭据 |
| `PATCH` | `/api/admin/providers/{id}/status` | 管理员 | 启用或停用提供商 |
| `DELETE` | `/api/admin/providers/{id}` | 管理员 | 软删除提供商及其模型路由 |
| `POST` | `/api/admin/providers/{id}/models/sync` | 管理员 | 从提供商同步模型并补充模型目录 |
| `POST` | `/api/admin/providers/{id}/models` | 管理员 | 从模型目录批量创建关联路由 |
| `GET` | `/api/admin/upstream-models` | 管理员 | 搜索和筛选模型目录 |
| `POST` | `/api/admin/upstream-models` | 管理员 | 创建目录模型和完整价格维度 |
| `PATCH` | `/api/admin/upstream-models/{id}` | 管理员 | 修改目录模型和价格维度 |
| `PATCH` | `/api/admin/upstream-models/{id}/status` | 管理员 | 启用或停用目录模型 |
| `DELETE` | `/api/admin/upstream-models/{id}` | 管理员 | 软删除目录模型及其模型路由 |
| `GET` | `/api/admin/model-routes` | 管理员 | 搜索和筛选模型路由 |
| `POST` | `/api/admin/model-routes` | 管理员 | 创建对外模型到上游模型的映射 |
| `PATCH` | `/api/admin/model-routes/{id}` | 管理员 | 修改上游模型映射和显示名称 |
| `PATCH` | `/api/admin/model-routes/{id}/status` | 管理员 | 启用或停用模型路由 |
| `DELETE` | `/api/admin/model-routes/{id}` | 管理员 | 软删除模型路由 |
| `GET` | `/api/admin/billing-groups` | 管理员 | 查看计费分组和用户数 |
| `POST` | `/api/admin/billing-groups` | 管理员 | 创建计费分组和倍率 |
| `PATCH` | `/api/admin/billing-groups/{id}` | 管理员 | 修改分组名称或倍率 |
| `PATCH` | `/api/admin/billing-groups/{id}/status` | 管理员 | 启用或停用非默认分组 |
| `DELETE` | `/api/admin/billing-groups/{id}` | 管理员 | 软删除未使用的非默认分组 |

控制台 `/api/*` 的写请求（除 `GET`、`HEAD` 和 `OPTIONS` 外）必须携带
`Origin`，并且必须匹配 `NOVRO_ALLOWED_ORIGINS` 中配置的完整来源；缺失或不匹配时
返回 `403 invalid_origin`。易支付异步通知路径是唯一例外，它不依赖浏览器来源，而是
校验商户 ID 和通知签名。反向代理必须保留浏览器的 `Origin` 请求头。模型兼容
`/v1/*` 使用 API Key 的机器请求不受这条浏览器来源规则影响。错误使用 `error.code`
与安全的中文提示，不返回 SQL、连接字符串、密码哈希或会话令牌。

每个 HTTP 请求都会返回 `X-Novro-Request-ID`。错误 JSON 还会返回顶层
`request_id`，控制台 API、网关鉴权失败和网关上游错误都使用同一 ID，便于结合结构化
日志排查。客户端提供的同名请求头不会被采纳。

修改用户角色和停用用户都受最后启用管理员保护。系统会在数据库事务内锁定启用的
管理员集合，不能通过角色降级或状态变更移除最后一个可用管理员。

本地注册、首次管理员初始化和管理员创建用户都要求 `username`、`email` 与 `password`；
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
模型目录配置为准。上游返回重定向时网关不会自动跟随到新地址，而是将响应转换为安全的
`502 upstream_error` 并释放本次预占余额，避免把提供商凭据带到未预期的主机。

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

API Key 属于单个用户，数据库只保存 SHA-256 哈希和前缀。第一版不引入项目 Key、组织
Key 或临时 Key。标准入口使用 `Authorization: Bearer`；Anthropic 客户端也可使用
`x-api-key`。

## 2. 模型列表

```http
GET /v1/models
```

返回当前网关可以使用的启用模型路由。模型名称是管理员配置的 `public_name`，响应不会
暴露上游 API Key 或加密配置。

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
- 按上游模型配置的普通输入、缓存命中、缓存创建（5 分钟/1 小时）、输出和按次价格，
  以人民币微元（1 元 = 1,000,000 微元）结算；用户计费分组倍率作用于汇总费用。
- 网关根据请求体大小、最大输出 token 和最高输入单价做保守余额预占；成功后按上游
  usage 结算并释放差额。实际费用超过预占时追加 `usage_settlement` 补扣流水，不截断。
- 供应商调用成功后，根据实际 usage 扣减用户余额。
- 余额不足时拒绝请求，不向供应商发起调用。
- 上游失败会释放预占；流式请求在结束或异常时记录最终用量，缺少 usage 时标记为估算。
- SSE 单行事件会分段透传；超过网关 usage 解析上限时不截断响应，但该次用量会标记为估算。
- 结算和失败退款遇到短暂存储错误时会用同一请求 ID 有限重试，业务冲突不会重复执行。
- 每次余额变化都写入余额流水，不能只更新余额总数。
- 钱包行在事务中锁定，避免并发请求共同花费同一份余额。

### 在线充值

- 充值金额使用与钱包相同的人民币微元存储，只接受精确到分的金额；管理员可以在系统上限内配置全局范围以及每个支付方式的最低金额。
- 支付方式包含显示名称、易支付 `type` 处理标识、图标、启用状态和最低充值金额；用户端只接收已启用方式。
- 管理员可配置预设充值金额和按门槛递增的赠送比例。订单分别保存用户支付金额和实际到账金额；易支付签名与回调始终核对支付金额，赠送只影响入账余额。
- 创建订单后返回易支付 `submit.php` 的签名表单字段，商户密钥不会发送给浏览器。
- 仅 `TRADE_SUCCESS` 通知会进入入账流程；商户 ID、MD5 签名、订单号、渠道和金额必须全部匹配。
- 充值订单、钱包余额、`top_up` 流水在同一数据库事务内更新。支付平台重复通知同一订单时只入账一次。
- 浏览器 `return_url` 只负责回到余额页，不能作为到账依据；到账状态以服务端异步通知为准。
- 管理员充值记录支持按订单号、网关流水号、用户、状态和支付方式筛选，不提供修改历史订单的入口。

### 用量字段兼容

网关读取 OpenAI/GLM 的 `prompt_tokens_details.cached_tokens`、DeepSeek 的
`prompt_cache_hit_tokens`/`prompt_cache_miss_tokens`、Kimi 的 `cached_tokens`，以及
Anthropic 的 `cache_read_input_tokens`、`cache_creation_input_tokens` 和 5 分钟/1 小时
明细。普通输入、缓存命中、缓存创建和输出分别乘各自单价，原始 Token 数、单价、分组
倍率、基础成本、最终费用和算法版本都会写入 `api_usages`，后续改价不会改变历史账单。

## 7. 错误响应

错误响应统一返回 JSON，并保留 OpenAI 风格的 `error` 对象。错误中不得泄露供应商密钥、
内部 URL 或数据库信息。错误响应包含顶层 `request_id`，上游非 2xx 会统一转换为
`502 upstream_error`。

## 8. Kimi / Moonshot 配置约定

Kimi 作为 OpenAI 兼容提供商接入，推荐基础地址为 `https://api.moonshot.cn/v1`。在
管理员控制台保存提供商凭据后，系统会先尝试同步模型；管理员再从模型目录多选模型，
将对外模型名映射到账户实际可用的 Kimi 模型 ID。凭据始终只在服务端使用；调用日志会记录模型、API Key 名称、Tokens、
费用和是否为估算值，不记录请求体、Authorization 头或上游密钥。
