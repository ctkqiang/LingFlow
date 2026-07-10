# LingFlow 技术栈

## 后端技术栈

### 核心语言
- **Go 1.21+** — 主编程语言，高性能并发支持
- 标准库为主，减少第三方依赖

### Web 框架
- **Gorilla WebSocket** — WebSocket 连接管理
- 原生 `net/http` — REST API 服务

### AWS 服务集成
- **AWS SDK for Go v2** — AWS 服务官方 SDK
- **Amazon S3** — 技能文件存储
- **Amazon Bedrock** — LLM 推理服务（Converse API）
- **Amazon Lambda** — 无服务器部署支持
- **AWS Secrets Manager** — 密钥管理（可选）

### 数据与事件
- **内存事件存储** (InMemory EventStore) — 事件溯源
- **EventBus** — 进程内事件总线
- **Aggregate Pattern** — 领域驱动设计聚合根

### 日志与配置
- **标准库 log/slog** — 结构化日志
- **环境变量配置** — 12-Factor 应用风格
- **dotenv 支持** — 开发环境 .env 文件

---

## 前端技术栈

### 核心框架
- **Vue 3** — 渐进式 JavaScript 框架
- **TypeScript 5+** — 类型安全
- **Composition API** — 组合式 API
- **`<script setup>`** — 语法糖

### 构建工具
- **Vite** — 下一代前端构建工具
- **Bun** — 快速的 JavaScript 运行时和包管理器

### 状态管理
- **Pinia** — Vue 官方状态管理库
- 模块化 Store 设计

### UI 样式
- **CSS Variables** — 主题变量系统
- 自定义主题色系统
- 响应式设计

### Markdown 渲染
- **marked** — Markdown 解析和渲染库

---

## 基础设施

### 容器化
- **Docker** — 容器化部署
- 多阶段构建优化镜像大小

### 编排
- **Kubernetes** — 容器编排
- **Helm**（可选）— K8s 包管理

### CI/CD
- **GitLab CI** — `.gitlab-ci.yml`
- **GitHub Actions** — CodeQL 安全扫描

### 代码托管
- GitHub
- GitCode
- Gitee
- GitLab

---

## 文档与设计

### 文档格式
- **Markdown** — 技术文档
- **HTML + CSS** — 文档站点和着陆页

### 图表工具
- **PlantUML** — 架构图、流程图
- 导出 PNG 格式

### UI 设计
- **Arco Design** — 字节跳动 UI 组件库（CDN）
- 赛博安全暗色霓虹美学风格

---

## 开发工具

### 代码质量
- **gofmt** — Go 代码格式化
- **golangci-lint**（推荐）— Go Lint 工具
- **ESLint**（推荐）— TypeScript/JavaScript Lint
- **Prettier**（推荐）— 代码格式化

### 测试
- **Go testing** — 后端单元测试
- **Vitest**（推荐）— 前端单元测试
- **Playwright**（推荐）— E2E 测试

### 安全工具
- **CodeQL** — 代码安全扫描
- **依赖审计** — 定期检查依赖漏洞

---

## 架构模式

### 后端架构
- **分层架构** — 连接层 / 服务层 / 事件层 / 模型层 / 工具层
- **事件溯源 (Event Sourcing)** — 不可变事件记录
- **CQRS**（部分实现）— 命令查询职责分离
- **Repository Pattern** — 数据访问抽象
- **Service Layer** — 业务逻辑层
- **Dependency Injection** — 依赖注入

### 前端架构
- **MVVM** — Model-View-ViewModel
- **组件化设计** — 可复用组件
- **单向数据流** — 状态管理模式

---

## 通信协议

- **WebSocket** — 实时双向通信
- **REST API** — 认证 Token 获取
- **JSON** — 消息序列化格式
- **Ping/Pong** — 心跳检测机制

---

## 安全机制

### 认证与授权
- **Token 认证** — WebSocket 连接验证
- **HMAC 签名**（预留）— 生产环境认证
- **Debug 模式** — 开发环境免认证

### 防护机制
- **Origin 检查** — 防止跨站 WebSocket 连接
- **IP 连接数限制** — 防止连接耗尽
- **速率限制** — 防止技能创建滥用
- **提示注入检测** — 输入/输出双层防护
- **Bedrock Guardrail**（可选）— AWS 原生防护
- **两阶段提交** — 安全创建技能文件
- **生产环境错误隐藏** — 不暴露内部细节
