# 第一版路线图

## 阶段 0：产品确认

- 确认 Novro 品牌和中文传播方式
- 确认用户、余额、Key、管理员四个核心边界
- 确认 OpenAI 和 Anthropic 三类 API 路径

## 阶段 1：基础工程

- [x] 创建 Go 服务工程
- [x] 创建 Next.js 和 shadcn/ui 管理界面
- [x] 初始化 Ent 和 MySQL 连接
- [x] 建立配置读取和基础日志

## 阶段 2：账号和 Key

- [x] 用户登录和会话
- [x] 管理员创建、启用、停用用户
- [x] 管理员重置用户密码
- 用户创建和撤销 API Key
- Key 哈希存储和创建时一次性展示

## 阶段 3：余额

- 钱包和余额流水
- 管理员手动调整余额
- API 调用前余额校验
- API 调用后的用量扣减

## 阶段 4：统一 API

- `/v1/models`
- `/v1/chat/completions`
- `/v1/responses`
- `/v1/messages`
- 非流式和 SSE 流式响应
- Kimi、GLM、DeepSeek 的服务端模型映射

## 阶段 5：验证和上线准备

- 用户、余额、Key 的权限测试
- 三种协议的非流式和流式测试
- 并发和余额一致性测试
- MySQL 备份和恢复演练
- API 文档和部署文档
