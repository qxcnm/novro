# Novro

国内大模型统一 API 网关。

> 一个接口，接入国内主流大模型。

Novro 面向需要在企业内部统一使用 Kimi、GLM、DeepSeek 等模型的团队，提供账号、余额、API Key、提供商和模型路由管理，并通过统一网关转发到官方或第三方上游。余额支持通过易支付协议在线充值；当前不包含套餐、订阅、组织和大型运营系统。

## 当前阶段

当前阶段已经完成：Go 模块化单体、Ent、版本化 MySQL 迁移、健康检查、用户名或邮箱登录与
管理员用户管理、API Key、加密提供商配置、模型目录、提供商模型同步、关联模型路由、计费分组、整数微元余额账本、易支付充值，以及
支持深色/浅色主题和响应式侧边导航的 Next.js 控制台。

统一 API 已提供 `/v1/models`、`/v1/chat/completions`、`/v1/responses` 和
`/v1/messages`。同一对外模型可以配置多条提供商路由；请求在启用渠道间轮询，并在当前
渠道遇到临时失败时重试一次，仍失败且尚未开始向客户端返回内容时切换下一条路由。成功后按实际命中上游 usage 的普通输入、
缓存命中、缓存创建和输出分别记录 token，并应用 API Key 所属计费分组倍率扣除余额；候选供应商只来自与该 Key 同分组的提供商。当前尚未实现
套餐、订阅、组织、项目和复杂供应商编排。

公共介绍页提供产品说明，`/models` 展示厂商官方牌价，`/docs` 是面向 API 使用者的
接入文档；运行中的可用模型以 `/v1/models` 和管理员模型路由为准。

## 已确定的技术方向

- 后端：Go `1.26.5`
- 前端：Next.js + TypeScript
- UI：原始 shadcn/ui
- 数据库：MySQL `8.4.9`
- ORM：Ent
- API：OpenAI Responses、OpenAI Chat Completions、Anthropic Messages

## 文档

- [开发环境、迁移与启动](docs/getting-started.md)
- [Docker 单应用部署](docs/docker-deployment.md)
- [生产部署、备份与恢复](docs/deployment.md)
- [工程规范](docs/engineering.md)
- [产品概述](docs/product-overview.md)
- [API 约定](docs/api.md)
- [数据模型](docs/data-model.md)
- [管理后台](docs/admin-console.md)
- [环境记录](docs/environment.md)
- [命名说明](docs/naming.md)
- [第一版路线图](docs/roadmap.md)

面向 API 使用者的接入说明请运行前端后访问 `/docs`；模型参数与官方牌价见
`/models`。仓库开发、数据库和部署信息不放入公开接入页面。

## 产品名称

产品名为 **Novro**，国内传播时读作“诺沃”。完整产品名为 **Novro Gateway**，代码仓库名为 `novro`。
