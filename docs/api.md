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
| `PATCH` | `/api/admin/users/{id}/status` | 管理员 | 启用或停用用户 |
| `POST` | `/api/admin/users/{id}/reset-password` | 管理员 | 重置密码并撤销旧会话 |

控制台写请求校验 `Origin`。错误使用 `error.code` 与安全的中文提示，不返回 SQL、
连接字符串、密码哈希或会话令牌。

OIDC 使用 Issuer Discovery、Authorization Code、PKCE、state 和 nonce。外部身份以
`issuer + subject` 唯一绑定，不根据邮箱自动合并已有本地账号。上游 access token、
ID Token 和 Client Secret 不发送给浏览器。

下面的 `/v1` 模型兼容 API 是后续阶段契约，当前尚未实现。

面向 API 使用者的完整示例由前端 `/docs` 提供；模型参数和厂商官方牌价由
`/models` 提供。本文件保留服务端契约与实现边界。

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

API Key 属于单个用户。第一版不引入项目 Key、组织 Key 或临时 Key。

## 2. 模型列表

```http
GET /v1/models
```

返回当前租户可以使用的模型。模型名称由 Novro 的服务端映射配置决定，第一版至少提供 Kimi、GLM 和 DeepSeek 的可用模型标识。

## 3. OpenAI Chat Completions

```http
POST /v1/chat/completions
```

兼容常见 OpenAI Chat Completions 请求。第一版需要覆盖：

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

兼容 OpenAI Responses 风格请求，至少覆盖文本输入、模型选择、流式输出和基本工具调用字段。Responses 与 Chat Completions 的内部模型请求可以复用同一套用户认证、余额扣减和供应商路由逻辑。

## 5. Anthropic Messages

```http
POST /v1/messages
```

兼容 Anthropic Messages 风格请求，至少覆盖：

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

- 请求进入网关后先验证用户、Key、模型和账户状态。
- 供应商调用成功后，根据实际使用量扣减用户余额。
- 余额不足时拒绝请求，不向供应商发起调用。
- 流式请求需要在结束或异常时记录最终用量。
- 每次余额变化都写入余额流水，不能只更新余额总数。

## 7. 错误响应

错误响应统一返回 JSON，并保留 OpenAI 风格的 `error` 对象；Anthropic 入口返回 Anthropic 客户端可以识别的错误结构。错误中不得泄露供应商密钥、内部 URL 或数据库信息。
