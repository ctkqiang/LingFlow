# LingFlow 项目规则

## 概述

本文档定义 LingFlow 项目的开发规范、代码组织约定和协作流程。所有贡献者必须遵守本规则。

## 项目结构约定

### 后端 (Go)

- 所有 Go 源代码位于项目根目录的 `internal/` 下
- 按领域分层：`connections/`、`services/`、`events/`、`models/`、`utilities/`
- 入口文件为 `main.go`
- 测试文件与源文件同目录，以 `_test.go` 结尾

### 前端 (Vue + TypeScript)

- 所有前端代码位于 `demo/` 目录
- 使用 MVVM 架构模式
- 组件位于 `src/components/`
- 状态管理位于 `src/stores/`
- 类型定义位于 `src/types/`
- API 封装位于 `src/api/`
- 样式位于 `src/styles/`

### 文档

- 项目主文档：`README.md`
- 文档站点：`docs/` 目录
- 图片资源：`docs/images/`
- 社区健康文件：`.github/`、`.gitcode/`、`.gitee/`、`.gitlab/`

### 部署配置

- Kubernetes 配置：`k8s/` 目录
- CI/CD 配置：`.gitlab-ci.yml`、`.github/workflows/`

## 代码风格约定

### Go 代码

- 使用标准 Go 格式化工具（`gofmt`）
- 包名使用小写，不使用下划线
- 结构体名使用 PascalCase
- 方法名使用 PascalCase（导出）或 camelCase（私有）
- 常量使用 PascalCase，分组声明
- 错误处理：函数返回最后一个值为 error，错误信息使用中文
- 日志：使用 `utilities/logger.go` 中的 Logger，日志输出为中文
- 每个包应有清晰的职责边界

### TypeScript / Vue 代码

- 使用 TypeScript 严格模式
- 组件使用 `<script setup lang="ts">` 语法
- Props 使用 `defineProps` 类型声明
- 组件文件名使用 PascalCase
- 组合式函数使用 camelCase，以 `use` 开头
- 所有类型定义集中在 `types/` 目录

### 命名约定

#### 后端
- WebSocket 消息类型：`TypeXxx` 常量
- 事件类型：`EventTypeXxx` 常量
- 命令类型：`CommandTypeXxx` 常量
- 服务接口：`XxxService`
- 管理器：`XxxManager`

#### 前端
- 组件：`XxxPanel`、`XxxList`、`XxxItem`、`XxxModal`
- Store：`useXxxStore`
- 组合式函数：`useXxx`

## Git 提交规范

### 提交信息格式

```
<类型>: <简短描述>

<详细描述>
```

### 类型说明

- `feat`: 新功能
- `fix: 修复 bug
- `docs`: 文档更新
- `style`: 代码格式调整
- `refactor`: 重构
- `test`: 测试相关
- `chore`: 构建/工具链相关

### 分支策略

- `main`: 主分支，稳定版本
- `develop`: 开发分支
- `feature/xxx`: 功能分支
- `bugfix/xxx`: 修复分支
- `hotfix/xxx`: 紧急修复分支

## 安全约定

### 代码安全

- 永远不要在代码中硬编码密钥、凭证
- 所有敏感配置通过环境变量注入
- 用户输入必须验证和转义
- 提示注入检测必须启用（生产环境）
- 错误信息不得暴露内部实现细节（生产环境）

### 依赖安全

- 定期更新依赖
- 使用安全扫描工具（如 CodeQL）
- 审核第三方依赖的安全性

## 文档约定

- 所有文档使用中文
- README 使用 Markdown 格式
- 架构图使用 PlantUML，导出为 PNG
- 文档中图片路径使用相对路径
- 不使用 emoji
- 使用精确的技术术语

## 测试约定

- 核心业务逻辑必须有单元测试
- 测试覆盖率目标：> 70%
- 测试文件与源文件同目录
- 测试用例应覆盖正常路径和异常路径

## 国际化 (i18n)

- 后端日志和错误信息使用中文（zh-CN）
- 前端界面使用中文（zh-CN）
- 预留国际化扩展能力

## 性能约定

- WebSocket 连接需支持心跳检测
- S3 操作使用合理的超时设置
- LLM 调用有超时保护
- 速率限制防止滥用
- 连接数限制防止资源耗尽

## 事件溯源约定

- 所有业务操作通过 Command 发起
- Aggregate 负责业务规则验证
- Event 是不可变的事实记录
- EventStore 持久化所有事件
- EventBus 负责事件通知
- Projection 通过事件增量构建状态
