# API 约定

## 当前控制台 API

本阶段已实现以下同源 Cookie 会话接口：

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| `POST` | `/api/auth/login` | 公开 | 登录并设置 HttpOnly 会话 Cookie |
| `GET` | `/api/auth/options` | 公开 | 返回可公开的注册、初始化和 OIDC 开关 |
| `POST` | `/api/auth/setup` | 公开 + 安装令牌 | 仅在空库中创建第一个管理员 |
| `POST` | `/api/auth/register` | 公开 + 配置开关 | 注册普通成员并建立会话 |
| `GET` | `/api/auth/oidc/start` | 公开 | 启动 OIDC Authorization Code + PKCE 流程 |
| `GET` | `/api/auth/oidc/callback` | 公开 | 验证 OIDC 回调并建立 Novro 会话 |
| `POST` | `/api/auth/logout` | 登录可选 | 撤销当前会话并清除 Cookie |
| `GET` | `/api/auth/me` | 登录 | 返回当前用户 |
| `GET` | `/api/admin/users` | 管理员 | 分页、搜索和状态筛选 |
| `POST` | `/api/admin/users` | 管理员 | 创建管理员或成员 |
| `PATCH` | `/api/admin/users/{id}` | 管理员 | 修改显示名称或角色 |
| `PATCH` | `/api/admin/users/{id}/status` | 管理员 | 启用或停用用户 |
| `POST` | `/api/admin/users/{id}/reset-password` | 管理员 | 重置密码并撤销旧会话 |
| `GET` | `/api/account/api-keys` | 登录 | 当前用户的 Key 元数据 |
| `POST` | `/api/account/api-keys` | 登录 | 创建 Key，完整值只返回一次 |
| `DELETE` | `/api/account/api-keys/{id}` | 登录 | 撤销当前用户的 Key |
| `GET` | `/api/account/balance` | 登录 | 当前余额和最近流水 |
| `GET` | `/api/account/usage` | 登录 | 当前用户最近调用用量 |
| `GET` | `/api/admin/api-keys` | 管理员 | 分页审计所有用户的 Key 元数据 |
| `POST` | `/api/admin/api-keys/{id}/revoke` | 管理员 | 撤销任意用户的 Key |
| `GET` | `/api/admin/users/{id}/balance` | 管理员 | 查看用户余额和流水 |
| `POST` | `/api/admin/users/{id}/balance-adjustments` | 管理员 | 原子调整余额并写入流水 |
| `GET` | `/api/admin/providers` | 管理员 | 搜索和筛选提供商 |
| `POST` | `/api/admin/providers` | 管理员 | 创建加密凭据配置 |
| `PATCH` | `/api/admin/providers/{id}` | 管理员 | 修改提供商配置或替换凭据 |
| `PATCH` | `/api/admin/providers/{id}/status` | 管理员 | 启用或停用提供商 |
| `GET` | `/api/admin/model-routes` | 管理员 | 搜索和筛选模型路由 |
| `POST` | `/api/admin/model-routes` | 管理员 | 创建对外模型到上游模型的映射 |
| `PATCH` | `/api/admin/model-routes/{id}` | 管理员 | 修改提供商、上游名称和单价 |
| `PATCH` | `/api/admin/model-routes/{id}/status` | 管理员 | 启用或停用模型路由 |

控制台写请求校验 `Origin`。错误使用 `error.code` 与安全的中文提示，不返回 SQL、
连接字符串、密码哈希或会话令牌。

每个 HTTP 请求都会返回 `X-Novro-Request-ID`。错误 JSON 还会返回顶层
`request_id`，控制台 API、网关鉴权失败和网关上游错误都使用同一 ID，便于结合结构化
日志排查。客户端提供的同名请求头不会被采纳。

修改用户角色和停用用户都受最后启用管理员保护。系统会在数据库事务内锁定启用的
管理员集合，不能通过角色降级或状态变更移除最后一个可用管理员。

OIDC 使用 Issuer Discovery、Authorization Code、PKCE、state 和 nonce。外部身份以
`issuer + subject` 唯一绑定，不根据邮箱自动合并已有本地账号。上游 access token、
ID Token 和 Client Secret 不发送给浏览器。

`/v1` 模型兼容 API 已实现基础非流式和 SSE 流式转发。上游请求会使用管理员配置的
提供商凭据，浏览器和 API 客户端永远不会收到上游密钥。面向 API 使用者的完整示例由
前端 `/docs` 提供；模型目录的官方牌价不是当前 Novro 结算价，实际结算单价以模型路由
配置为准。

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
- 按模型路由配置的输入/输出价，以人民币微元（1 元 = 1,000,000 微元）结算。
- 网关根据请求体大小和最大输出 token 做保守余额预占；成功后按上游 usage 结算并释放差额。
- 供应商调用成功后，根据实际 usage 扣减用户余额。
- 余额不足时拒绝请求，不向供应商发起调用。
- 上游失败会释放预占；流式请求在结束或异常时记录最终用量，缺少 usage 时标记为估算。
- 每次余额变化都写入余额流水，不能只更新余额总数。
- 钱包行在事务中锁定，避免并发请求共同花费同一份余额。

## 7. 错误响应

错误响应统一返回 JSON，并保留 OpenAI 风格的 `error` 对象。错误中不得泄露供应商密钥、
内部 URL 或数据库信息。错误响应包含顶层 `request_id`，上游非 2xx 会统一转换为
`502 upstream_error`。
