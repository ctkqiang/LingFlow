# 贡献指南

首先，感谢你考虑为 LingFlow 做出贡献！正是因为像你这样的开发者，开源社区才能不断成长。

本文档详细描述了参与 LingFlow 开发的完整流程、规范和要求。请在提交任何代码或 issue 之前仔细阅读。

---

## 目录

- [行为准则](#行为准则)
- [前置要求](#前置要求)
- [开发环境搭建](#开发环境搭建)
- [项目结构与代码规范](#项目结构与代码规范)
- [开发工作流](#开发工作流)
- [提交规范](#提交规范)
- [Pull Request 流程](#pull-request-流程)
- [Issue 报告指南](#issue-报告指南)
- [测试要求](#测试要求)
- [技能贡献指南](#技能贡献指南)
- [文档贡献指南](#文档贡献指南)
- [社区讨论](#社区讨论)

---

## 行为准则

参与本项目的所有贡献者必须遵守 [行为准则](CODE_OF_CONDUCT.md)。请始终保持尊重和包容的态度。如遇到不当行为，请通过 Issue 或邮件举报。

---

## 前置要求

### 必需工具

| 工具 | 最低版本 | 用途 | 安装方式 |
|------|----------|------|----------|
| [Go](https://go.dev/dl/) | 1.26.1 | 编译与运行 | `brew install go` 或官网下载 |
| [Git](https://git-scm.com/) | 2.30+ | 版本控制 | `brew install git` |
| [AWS CLI](https://aws.amazon.com/cli/) | 2.x | AWS 资源管理 | `brew install awscli` |
| [PlantUML](https://plantuml.com/) | 1.2024+ | 架构图渲染 | `brew install plantuml` |

### 推荐工具

| 工具 | 用途 |
|------|------|
| [golangci-lint](https://golangci-lint.run/) | Go 代码静态检查 |
| [gofumpt](https://github.com/mvdan/gofumpt) | 代码格式化（比 gofmt 更严格） |
| [Postman](https://www.postman.com/) / [Reqable](https://reqable.com/) | WebSocket 接口测试 |
| [Docker](https://www.docker.com/) | 容器化部署测试 |

### AWS 资源要求

贡献者在开发时需要以下 AWS 资源（可使用 Mock 模式跳过）：

- **S3 存储桶**：用于技能文件存储（可使用本地 MinIO 替代）
- **Bedrock 模型访问**：用于 LLM 推理（开发时可用 `LLM_MOCK_MODE=true` 跳过）

---

## 开发环境搭建

### 步骤 1：Fork 并克隆仓库

```bash
# Fork 仓库后克隆你的 Fork
git clone https://github.com/YOUR_USERNAME/LingFlow.git
cd LingFlow

# 添加上游仓库
git remote add upstream https://github.com/ORIGINAL_OWNER/LingFlow.git
```

### 步骤 2：安装依赖

```bash
go mod download
go mod tidy
```

### 步骤 3：配置环境

```bash
cp .env.example .env

# 最小开发配置（Mock 模式，无需 AWS 凭证）
# 编辑 .env，设置以下内容：
# LLM_MOCK_MODE=true
# MODE=development
```

### 步骤 4：验证开发环境

```bash
# 编译检查
go build ./...

# 运行测试
go test ./...

# 启动服务
go run main.go

# 验证服务
curl -X POST http://localhost:4030/api/auth/token \
  -H "Content-Type: application/json" \
  -d '{"user_id": "dev-test"}'
```

---

## 项目结构与代码规范

### 目录结构

```
LingFlow/
├── main.go                    # 应用入口
├── internal/
│   ├── connections/           # WebSocket 连接管理
│   ├── events/                # 事件溯源（Event Sourcing）
│   ├── models/                # 数据模型定义
│   ├── services/              # 业务逻辑层
│   └── utilities/             # 工具函数
├── docs/                      # 文档资源
│   └── images/                # 图片与 PlantUML 源文件
└── .github/                   # GitHub 配置（模板、CI/CD）
```

### Go 代码规范

**1. 命名规范**

| 类型 | 规范 | 示例 |
|------|------|------|
| 包名 | 全小写，单个词 | `connections`, `services` |
| 文件名 | 全小写，下划线分隔 | `chat_handler.go`, `s3_skill_loader.go` |
| 导出标识符 | 驼峰，首字母大写 | `WebSocketManager`, `LoadAllSkills` |
| 未导出标识符 | 驼峰，首字母小写 | `handleHeartbeat`, `extractBucket` |
| 常量 | 驼峰或全大写下划线 | `MaxConnections`, `defaultSkillsS3Prefix` |
| 接口 | 以 `er` 结尾或描述行为 | `EventStore`, `LLMService`, `SkillLoader` |

**2. 代码格式**

```bash
# 提交前必须格式化
gofmt -w .
# 或使用更严格的
gofumpt -w .
```

**3. 错误处理**

```go
// ✅ 正确：错误包装，保留上下文
if err != nil {
    return fmt.Errorf("S3 HeadObject 失败 (bucket=%s key=%s region=%s): %w",
        loader.bucket, skillKey, loader.region, err)
}

// ❌ 错误：丢失上下文
if err != nil {
    return err
}

// ✅ 正确：生产环境隐藏敏感信息
if mode == "production" {
    return errors.New("操作被拒绝: 服务暂时不可用")
}

// ❌ 错误：暴露内部信息
return fmt.Errorf("数据库连接失败: %s@%s:%d", user, host, port)
```

**4. 日志规范**

```go
// 使用项目统一的日志工具
utilities.LogProgress("ChatHandler", "processUserChat",
    "正在处理用户消息")

// 日志级别使用规范：
// DEBUG   - 开发调试信息
// VERBOSE - 每个步骤的详细参数
// INFO    - 服务启动、配置加载、常规操作
// WARN    - 非致命异常、降级处理
// ERROR   - 致命错误、需要人工干预
```

**5. 并发安全**

```go
// 使用 sync.Mutex 保护共享状态
type ConnectionManager struct {
    mu          sync.RWMutex
    connections map[string]*Connection
}

// 读操作使用 RLock
func (m *ConnectionManager) Get(id string) *Connection {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.connections[id]
}

// 写操作使用 Lock
func (m *ConnectionManager) Set(id string, conn *Connection) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.connections[id] = conn
}

// 使用非阻塞发送，防止 goroutine 泄漏
select {
case ch <- message:
default:
    // 通道满，丢弃或记录日志
}
```

---

## 开发工作流

### 分支策略

```
main          ← 稳定发布分支，只接受 PR 合并
develop       ← 开发集成分支（如使用）
feature/*     ← 新功能分支
fix/*         ← Bug 修复分支
docs/*        ← 文档更新分支
refactor/*    ← 代码重构分支
```

### 分支命名规范

```bash
# 功能分支
feature/add-conversation-history
feature/semantic-skill-search

# Bug 修复
fix/websocket-memory-leak
fix/s3-region-mismatch

# 文档
docs/update-readme
docs/add-deployment-guide

# 重构
refactor/event-store-interface
```

### 开发流程

```bash
# 1. 同步上游
git checkout main
git pull upstream main

# 2. 创建功能分支
git checkout -b feature/your-feature-name

# 3. 开发并提交
# ... 编写代码 ...
git add <files>
git commit -m "feat: 添加会话历史持久化功能"

# 4. 同步上游变更（如果有）
git fetch upstream
git rebase upstream/main

# 5. 推送到你的 Fork
git push origin feature/your-feature-name

# 6. 创建 Pull Request
```

---

## 提交规范

本项目采用 [约定式提交](https://www.conventionalcommits.org/zh-hans/) 规范。

### 提交消息格式

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Type 类型

| Type | 说明 | 示例 |
|------|------|------|
| `feat` | 新功能 | `feat(skills): 添加语义搜索功能` |
| `fix` | Bug 修复 | `fix(wss): 修复心跳超时断开问题` |
| `docs` | 文档更新 | `docs(readme): 更新部署指南` |
| `style` | 代码格式调整 | `style: gofmt 格式化` |
| `refactor` | 重构 | `refactor(events): 抽象 EventStore 接口` |
| `test` | 测试相关 | `test(services): 添加 skill_executor 测试` |
| `chore` | 构建/工具变更 | `chore: 更新 Go 依赖版本` |
| `ci` | CI/CD 变更 | `ci: 添加 GitHub Actions 工作流` |
| `perf` | 性能优化 | `perf(wss): 优化连接池并发性能` |

### Scope 范围

| Scope | 对应模块 |
|-------|----------|
| `wss` | WebSocket 连接管理 |
| `chat` | 聊天消息处理 |
| `skills` | 技能系统 |
| `events` | 事件溯源 |
| `llm` | LLM 服务 |
| `s3` | S3 存储 |
| `auth` | 认证授权 |
| `heartbeat` | 心跳机制 |
| `readme` | README 文档 |
| `security` | 安全相关 |

### 示例

```bash
# 功能
git commit -m "feat(skills): 添加基于嵌入向量的语义技能检索

使用 AWS Bedrock Embedding 模型对技能描述生成向量，
支持余弦相似度匹配，提升技能检索准确率。

Closes #42"

# Bug 修复
git commit -m "fix(s3): 修复 ARN 格式存储桶名称解析失败

当 SKILLS_S3_BUCKET 配置为 ARN 格式时（arn:aws:s3:::bucket），
extractBucketFromARN 函数未能正确提取 bucket 名称，
导致 S3 操作返回 NoSuchBucket 错误。

Fixes #87"

# 文档
git commit -m "docs(readme): 添加 Docker 部署指南和架构图"
```

---

## Pull Request 流程

### 提交前检查清单

- [ ] 代码通过 `go build ./...` 编译
- [ ] 代码通过 `gofmt -l .` 检查（无输出）
- [ ] 代码通过 `go vet ./...` 检查
- [ ] 新功能包含对应的单元测试
- [ ] 所有测试通过 `go test ./... -v`
- [ ] 提交消息符合约定式提交规范
- [ ] 更新了相关文档（如有必要）
- [ ] 没有硬编码的凭证或敏感信息
- [ ] 没有引入不必要的依赖

### PR 标题规范

```
<type>(<scope>): <简短描述>
```

示例：
- `feat(skills): 添加语义搜索功能`
- `fix(wss): 修复心跳超时断开问题`
- `docs: 完善部署指南`

### PR 描述要求

请使用 [PR 模板](.github/PULL_REQUEST_TEMPLATE.md) 填写以下内容：

1. **变更说明**：清晰描述本 PR 做了什么以及为什么
2. **变更类型**：新功能 / Bug 修复 / 重构 / 文档更新
3. **测试方式**：如何验证变更的正确性
4. **破坏性变更**：是否影响现有功能
5. **关联 Issue**：`Closes #123`、`Fixes #456`

### 审查流程

```
1. 提交 PR
   ↓
2. 自动 CI 检查（编译、测试、lint）
   ↓ 通过
3. 代码审查（至少 1 位维护者 Review）
   ↓ 通过
4. 如有修改意见，贡献者更新代码
   ↓
5. 最终批准（Approve）
   ↓
6. Squash Merge 到 main
   ↓
7. 删除功能分支
```

### 审查标准

维护者在审查时会关注以下方面：

| 维度 | 要求 |
|------|------|
| **功能正确性** | 代码实现的功能与描述一致，边界情况处理得当 |
| **测试覆盖** | 新功能必须有测试，覆盖核心路径和边界情况 |
| **代码质量** | 命名清晰、逻辑简洁、无冗余代码、错误处理完善 |
| **安全性** | 无硬编码凭证、无注入风险、生产模式隐藏敏感信息 |
| **性能** | 无明显的性能问题（如不必要的锁竞争、内存泄漏） |
| **文档** | 公开 API 有注释、行为变更有文档记录 |
| **兼容性** | 不破坏现有接口和配置（或提供迁移说明） |

---

## Issue 报告指南

### Bug 报告

提交 Bug 报告前请先搜索现有 Issue，避免重复。

使用 [Bug 报告模板](.github/ISSUE_TEMPLATE/bug_report.md)，包含以下信息：

1. **环境信息**：Go 版本、操作系统、部署方式
2. **复现步骤**：详细到可直接操作的程度
3. **预期行为**：你认为应该发生什么
4. **实际行为**：实际发生了什么（附日志/截图）
5. **配置信息**：`.env` 中的相关配置（**隐藏敏感值**）

### 功能请求

使用 [功能请求模板](.github/ISSUE_TEMPLATE/feature_request.md)，包含：

1. **问题背景**：当前遇到什么问题或限制
2. **期望方案**：你希望看到什么功能
3. **替代方案**：是否考虑过其他实现方式
4. **使用场景**：该功能在什么场景下使用

### 好的 Issue 标题示例

```
[Bug] S3 HeadObject 返回 403 当 SKILLS_S3_BUCKET 使用 ARN 格式
[Feature] 支持会话历史持久化到 DynamoDB
[Enhancement] 技能检索增加基于嵌入向量的语义搜索
```

---

## 测试要求

### 单元测试

```bash
# 运行所有测试
go test ./...

# 运行特定包测试
go test ./internal/services/...

# 详细输出
go test -v ./...

# 生成覆盖率报告
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### 测试编写规范

```go
// 文件命名：xxx_test.go，与被测文件同目录
// 函数命名：TestXxx_YyyScenario

func TestSkillExecutor_WhenSkillMatched_ReturnsResponse(t *testing.T) {
    // Given - 准备测试数据
    registry := NewSkillRegistry()
    registry.Register(&Skill{
        Identifier:  "/trade_analysis",
        DisplayName: "交易分析",
        Keywords:    []string{"交易", "分析"},
    })

    executor := NewSkillExecutor(registry, mockLLM)

    // When - 执行被测方法
    result, err := executor.Execute(context.Background(), "分析交易行情")

    // Then - 验证结果
    assert.NoError(t, err)
    assert.Equal(t, "/trade_analysis", result.SkillUsed.Identifier)
    assert.NotEmpty(t, result.Content)
}
```

### 测试覆盖要求

| 模块 | 最低覆盖率 |
|------|-----------|
| `internal/services/` | 70% |
| `internal/events/` | 75% |
| `internal/connections/` | 60% |
| `internal/models/` | 80% |
| `internal/utilities/` | 70% |

### 集成测试

涉及 AWS 资源的操作需编写集成测试，使用构建标签隔离：

```go
//go:build integration

package services

func TestS3SkillLoader_Integration(t *testing.T) {
    // 需要 AWS 凭证和真实 S3 存储桶
    // 使用 SKILLS_S3_BUCKET=integration-test-bucket
}
```

```bash
# 运行集成测试
go test -tags=integration ./internal/services/...
```

---

## 技能贡献指南

技能是 LingFlow 的核心资产。你可以通过以下方式贡献技能：

### 提交新技能

1. 按照 [README.md](README.md#技能系统) 中的技能 Markdown 模板编写技能文件
2. 将技能文件放入 `skills/` 目录
3. 提交 PR，标题格式：`feat(skills): 添加 <技能名称> 技能`

### 技能质量标准

| 维度 | 要求 |
|------|------|
| **描述清晰** | `description` 字段准确概括技能能力 |
| **关键词完整** | `keywords` 覆盖用户可能的查询词 |
| **角色定义** | 明确 LLM 在使用该技能时的专家角色 |
| **执行步骤** | 提供清晰的处理流程 |
| **输出格式** | 定义规范化的响应结构 |
| **约束规则** | 明确限制和禁止行为 |
| **触发示例** | 提供至少 3 个使用示例 |
| **错误处理** | 定义异常情况的处理方式 |

### 技能文件示例

```markdown
# 技能显示名称

description: 一句话描述技能核心能力
category: 分类名称
keywords: 关键词1, 关键词2, 关键词3

## 角色定义
[专家角色描述]

## 核心能力
1. 能力一
2. 能力二

## 执行步骤
1. 步骤一
2. 步骤二

## 输出格式规范
[格式定义]

## 约束与规则
1. 规则一
2. 规则二

## 触发示例
- 基础用例: "示例查询"
- 进阶用例: "复杂查询"
- 边界情况: "边界查询"

## 错误处理
[异常处理方案]
```

---

## 文档贡献指南

### 文档类型

| 类型 | 位置 | 说明 |
|------|------|------|
| 项目文档 | `README.md` | 项目概览、快速开始 |
| 架构文档 | `docs/` | 深入技术细节 |
| API 文档 | `README.md#api-参考` | 接口规范 |
| 代码注释 | 源文件内 | 导出函数/类型必须有注释 |
| 变更日志 | `CHANGELOG.md` | 版本变更记录 |

### 架构图更新

如需更新架构图，请修改 PlantUML 源文件并重新生成 PNG：

```bash
# 修改 .puml 文件后重新生成 PNG
plantuml docs/images/architecture.puml -o ./
plantuml docs/images/message_flow.puml -o ./

# 提交时同时提交 .puml 和 .png 文件
git add docs/images/architecture.puml docs/images/architecture.png
```

### 代码注释规范

```go
// SkillExecutor 负责技能检索、上下文注入和 LLM 调用的编排。
//
// 处理流程：
//   1. 从 SkillRegistry 检索匹配技能
//   2. 将技能上下文注入 System Prompt
//   3. 调用 LLMService 生成响应
//   4. 格式化响应并返回
type SkillExecutor struct {
    registry  SkillRegistry
    llm       LLMService
    eventStore EventStore
}

// Execute 执行技能匹配和 LLM 调用。
// 返回包含响应内容、使用技能和性能指标的 Result。
// 如果没有匹配的技能，使用默认 System Prompt 调用 LLM。
func (e *SkillExecutor) Execute(ctx context.Context, message string) (*Result, error) {
    // ...
}
```

---

## 社区讨论

- **GitHub Discussions**：用于技术讨论、问答和想法分享
- **GitHub Issues**：用于 Bug 报告和功能请求
- **Pull Requests**：用于代码贡献

### 沟通原则

1. **保持尊重**：尊重不同观点和经验水平
2. **清晰表达**：提供足够的上下文信息
3. **建设性反馈**：提出问题时附带建议
4. **耐心等待**：维护者可能在业余时间审查，请耐心等待

---

## 维护者指南

### 版本发布流程

```
1. 更新 CHANGELOG.md
2. 创建版本标签: git tag -a v1.x.0 -m "Release v1.x.0"
3. 推送标签: git push origin v1.x.0
4. 创建 GitHub Release，附上变更说明
```

### 版本号规范

遵循 [语义化版本](https://semver.org/lang/zh-CN/)：

```
MAJOR.MINOR.PATCH
   1     2     3

MAJOR - 不兼容的 API 变更
MINOR - 向下兼容的功能新增
PATCH - 向下兼容的 Bug 修复
```

---

## 致谢

感谢每一位为 LingFlow 做出贡献的开发者。你的每一行代码、每一个 Issue、每一篇文档都让这个项目变得更好。

<div align="center">

**让我们一起构建更好的 AI 技能驱动聊天服务框架！**

</div>
