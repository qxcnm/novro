# Novro

国内大模型统一 API 网关。

> 一个接口，接入国内主流大模型。

Novro 面向需要在企业内部统一使用 Kimi、GLM、DeepSeek 等模型的团队，提供简洁的多租户账号管理、余额管理和 API Key 管理能力。第一版优先把账号和调用入口做稳定，不包含支付、复杂计费或大型运营系统。

## 当前阶段

第一版基础工程已经建立：Go 模块化单体、Ent、版本化 MySQL 迁移、健康检查、
用户登录与管理员用户管理，以及支持深色/浅色主题的 Next.js 控制台。

当前尚未实现余额、API Key、统一模型 API、支付、套餐、组织、项目和复杂供应商后台。

公共介绍页提供产品说明，`/models` 展示模型与厂商官方牌价，`/docs` 是面向 API
使用者的接入文档。尚未交付的模型调用能力统一标记为“即将开放”。

## 已确定的技术方向

- 后端：Go `1.26.5`
- 前端：Next.js + TypeScript
- UI：原始 shadcn/ui
- 数据库：MySQL `8.4.9`
- ORM：Ent
- API：OpenAI Responses、OpenAI Chat Completions、Anthropic Messages

## 文档

- [开发环境、迁移与启动](docs/getting-started.md)
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
