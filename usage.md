# LingFlow 使用指南

## 概述

LingFlow 是一个基于 WebSocket 的 AI 聊天服务，支持从 AWS S3 动态加载技能，并通过多种消息类型流式返回响应。支持本地/服务器模式和 AWS Lambda 模式。

## 快速开始

### 本地开发

```bash
cp .env.example .env
# 编辑 .env 配置你的 AWS 凭证和参数
go run main.go
```

### 构建

```bash
go build -o lingflow main.go
./lingflow
```

### 运行测试

```bash
go test ./...
```

## 环境变量

### 核心配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `RUNTIME_MODE` | 运行模式：`local` 或 `cloud` | 自动检测 |
| `WSS_ADDR` | WebSocket 监听地址 | `:4030` |
| `LOG_LEVEL` | 日志级别：DEBUG, INFO, WARN, ERROR, VERBOSE | INFO |

### AWS 配置

| 变量 | 说明 | 必填 |
|------|------|------|
| `AWS_REGION` | Bedrock 和 S3 的 AWS 区域 | 是 |
| `BEDROCK_MODEL_ID` | Bedrock 模型 ID | 是 |
| `SKILLS_S3_BUCKET` | 存储技能文件的 S3 存储桶名称 | 是 |
| `SKILLS_S3_PREFIX` | S3 中技能文件的前缀 | (空) |

### 安全配置

| 变量 | 说明 | 必填 |
|------|------|------|
| `WSS_CERT_FILE` | TLS 证书文件路径 | 生产环境 |
| `WSS_KEY_FILE` | TLS 私钥文件路径 | 生产环境 |
| `WSS_AUTH_SECRET` | WebSocket Token 认证的 HMAC 密钥（同时控制 REST 认证接口是否启用） | 生产环境 |
| `AUTH_API_KEY` | REST 认证接口的 API Key（设置后客户端必须提供才能获取 token） | 生产环境 |
| `AUTH_TOKEN_TTL` | 签发的 Token 有效期，如 `24h`、`72h` | 24h |
| `WSS_ALLOWED_ORIGINS` | 允许的 Origin 列表，逗号分隔 | 生产环境 |
| `WSS_MAX_CONNECTIONS_PER_IP` | 单 IP 最大连接数 | 10 |
| `WSS_ALLOW_ALL_ORIGINS` | 允许所有 Origin（仅调试用） | false |

### 心跳配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `WSS_HEARTBEAT_INTERVAL` | 服务端 Ping 间隔 | 30秒 |
| `WSS_HEARTBEAT_TIMEOUT` | 连接空闲超时 | 90秒 |
| `WSS_HEARTBEAT_WRITE_TIMEOUT` | 写入操作超时 | 10秒 |

### AWS Secrets Manager（可选）

| 变量 | 说明 |
|------|------|
| `SECRET_ARN` | 密钥的完整 ARN |
| `SECRET_NAME` | 密钥名称 |

### S3 配置（可选）

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `S3_ENV_BUCKET` | 存储 .env 文件的 S3 存储桶 | (空) |
| `S3_ENV_KEY` | .env 文件在 S3 中的键名 | `.env` |

## WebSocket 协议

### 连接地址

```
ws://localhost:4030/chat/{会话ID}
wss://your-domain.com/chat/{会话ID}?token=<认证令牌>
```

### 认证

生产环境需要携带有效的 HMAC 令牌：
- 查询参数：`?token=<token>`
- 请求头：`Authorization: Bearer <token>`

**令牌格式**：`base64url(用户ID|过期时间戳).hex(hmacSha256)`

### 消息类型

#### 客户端 → 服务端

**用户聊天消息**
```json
{
  "type": "user_chat",
  "data": {
    "id": 1,
    "user_id": "user-123",
    "message": "我想分析这笔交易"
  },
  "timestamp": "2026-07-09T10:30:00Z"
}
```

**心跳 Ping**
```json
{
  "type": "heartbeat_chat",
  "data": {
    "action": "ping",
    "nonce": "abc123",
    "timestamp": "2026-07-09T10:30:00Z"
  },
  "timestamp": "2026-07-09T10:30:00Z"
}
```

#### 服务端 → 客户端

**系统思考**（技能匹配阶段）
```json
{
  "type": "system_thinking",
  "data": {
    "phase": "skill_selection",
    "skill_matches": [
      {
        "skill_identifier": "/trade_analysis",
        "confidence": 0.95
      }
    ],
    "selected_skill": {
      "skill_identifier": "/trade_analysis",
      "confidence": 0.95
    },
    "thought": "正在匹配用户查询与可用技能..."
  },
  "timestamp": "2026-07-09T10:30:01Z"
}
```

**系统思考**（LLM 生成阶段）
```json
{
  "type": "system_thinking",
  "data": {
    "phase": "llm_generation",
    "thought": "正在调用 Bedrock 生成响应..."
  },
  "timestamp": "2026-07-09T10:30:02Z"
}
```

**系统响应**
```json
{
  "type": "system_response",
  "data": {
    "content": "这是你的交易分析结果...",
    "skill_used": {
      "skill_identifier": "/trade_analysis"
    },
    "finish_reason": "stop",
    "tokens_used": 150,
    "latency_ms": 2500
  },
  "timestamp": "2026-07-09T10:30:03Z"
}
```

**心跳 Pong**
```json
{
  "type": "heartbeat_chat",
  "data": {
    "action": "pong",
    "nonce": "abc123",
    "timestamp": "2026-07-09T10:30:00Z",
    "latency": 50
  },
  "timestamp": "2026-07-09T10:30:00Z"
}
```

## S3 技能加载

### 技能文件格式

技能以 Markdown 文件形式存储在 S3 中：
- 文件名：`trade_analysis.md` → 技能标识：`/trade_analysis`
- 内容：必须符合 LingFlow 技能 schema

### 技能文件示例（`trade_analysis.md`）

```markdown
---
skill_identifier: /trade_analysis
skill_display_name: 交易分析
skill_description: 分析交易模式和市场数据
skill_category: 金融
search_keywords: ["交易", "分析", "市场", "股票"]
schema_version: 1.0
---

## 指令

你是一名交易分析专家。分析用户的查询并提供可行的洞察。

## 参数

- timeframe: 分析时间范围（日、周、月）
- indicators: 使用的技术指标
```

## 安全

### 完整认证流程

```
客户端                                    服务端
   |  1. POST /api/auth/token            |
   |     { user_id }                     |
   |------------------------------------>|
   |                                     | 开发模式：直接返回 token
   |                                     | 生产模式：执行用户自定义认证逻辑
   |  2. { token, user_id }              |
   |<------------------------------------|
   |                                     |
   |  3. GET /chat/uuid?token=<token>    |
   |     Upgrade: websocket              |
   |------------------------------------>|
   |                                     | 验证 Token 格式/有效性
   |  4. 101 Switching Protocols         |
   |<------------------------------------|
```

### 开发模式（MODE=development，默认）

**Step 1: 获取 Token**

请求：
```bash
curl -X POST http://localhost:4030/api/auth/token \
  -H "Content-Type: application/json" \
  -d '{"user_id": "my-user"}'
```

响应（200 OK）：
```json
{
  "token": "debug-token-my-user",
  "expires_at": 0,
  "user_id": "my-user",
  "ttl": "unlimited"
}
```

**Step 2: 使用 Token 连接 WebSocket**

查询参数方式：
```
ws://localhost:4030/chat/my-session-id?token=debug-token-my-user
```

Authorization 请求头方式：
```
Authorization: Bearer debug-token-my-user
```

**Step 3: 发送聊天消息**

连接成功后发送 WebSocket 消息：
```json
{
  "type": "user_chat",
  "data": {
    "id": 1,
    "user_id": "my-user",
    "message": "Hello LingFlow"
  },
  "timestamp": "2026-07-09T10:30:00Z"
}
```

### 生产模式（MODE=production）

当设置 `MODE=production` 时，需要在以下两个文件中实现真实验证逻辑：

**1. REST 认证接口** — [auth_handler.go](file:///internal/services/auth_handler.go) 的 `handleProductionAuth()` 方法：
- 验证客户端提供的凭证（API Key、用户名密码等）
- 查询数据库验证用户身份
- 生成安全的 Token（建议使用 JWT 或 HMAC）

**2. WebSocket 认证** — [wss.go](file:///internal/connections/wss.go) 的 `authenticateWebSocketProduction()` 方法：
- 验证 Token 的签名和过期时间
- 提取用户 ID

### 运行模式配置

| 环境变量 | 值 | 说明 |
|----------|-----|------|
| MODE | development | 默认，开发模式，简化认证 |
| MODE | production | 生产模式，执行用户自定义认证逻辑 |

`.env` 配置示例：
```bash
# 默认开发模式
MODE=development

# 切换到生产模式
# MODE=production
```

### 错误情况

| 场景 | HTTP 状态码 | 说明 |
|------|-------------|------|
| 获取 Token 时缺少 user_id | 400 | 请求体格式错误或缺少参数 |
| 连接 WebSocket 时缺少 token | 401 | 需要先调用 POST /api/auth/token |
| 开发模式下 token 格式不正确 | 401 | 必须是 debug-token-* 格式 |

### Postman 测试步骤

1. **新建 HTTP 请求** → POST → `http://localhost:4030/api/auth/token`
   - Headers: `Content-Type: application/json`
   - Body (raw JSON): `{"user_id": "test-user"}`
   - 发送后复制响应中的 `token` 值

2. **新建 WebSocket 请求** → `ws://localhost:4030/chat/test-session?token=debug-token-test-user`
   - 点击 Connect 连接

3. **发送消息** → 输入 JSON 消息并发送

### Reqable 测试步骤

1. **新建 HTTP 请求** → POST → `http://localhost:4030/api/auth/token`
   - Body: JSON `{"user_id": "test-user"}`
   - 发送获取 token

2. **新建 WebSocket 请求** → `ws://localhost:4030/chat/test-session`
   - 添加请求参数：`token=debug-token-test-user`
   - 点击连接

3. **发送消息** → 输入 JSON 消息并发送

## 事件溯源

LingFlow 使用事件溯源管理聊天会话：

**事件类型**：
- `chat_session_connected` — 会话连接
- `chat_session_disconnected` — 会话断开
- `chat_message_received` — 消息已接收
- `chat_message_processed` — 消息已处理
- `chat_message_processing_failed` — 消息处理失败
- `chat_message_broadcasted` — 消息已广播
- `heartbeat_ping_sent` — 心跳 Ping 已发送
- `heartbeat_ping_received` — 心跳 Ping 已接收
- `heartbeat_pong_sent` — 心跳 Pong 已发送
- `heartbeat_pong_received` — 心跳 Pong 已接收
- `heartbeat_timeout` — 心跳超时

**事件存储**：默认使用内存存储。生产环境请替换为持久化存储。

## 运行模式

### 服务器模式（本地/EC2）

```bash
export RUNTIME_MODE=server
go run main.go
```

### Lambda 模式（AWS）

```bash
export RUNTIME_MODE=lambda
# 部署为 Lambda 函数，配合 API Gateway WebSocket 集成
```

### 自动模式

自动检测 AWS Lambda 运行时（`AWS_LAMBDA_RUNTIME_API` 环境变量）并自动切换。

## 日志格式

所有日志采用 LingFlow 结构化格式，输出中文消息：

```
[LingFlow] [2026-07-09 10:30:00] [INFO] [LingFlow@20260709:10:30:00CST]::Services:: (Services:serveWebSocketHTTPServer>>TASK-001::serveWebSocketHTTPServer)
  | Status   : IN_PROGRESS
  | Type     : ACTION
  | Memory   : 12.34MB
  | Routine  : TASK-001
  | Elapsed  : 0μs
  | Progress : 监听 WebSocket endpoint
  | addr     : :4030
  | protocol : ws
  | path     : /chat/{uuid}
```

## 错误处理

### WebSocket 连接错误

- **401 Unauthorized**：认证令牌无效或缺失
- **400 Bad Request**：连接标识符无效
- **429 Too Many Requests**：IP 连接数超限
- **403 Forbidden**：Origin 不在白名单

### 优雅关闭

发送 SIGINT 或 SIGTERM 信号触发优雅关闭：
- 停止接收新连接
- 最多等待 10 秒处理进行中的消息
- 关闭所有连接

## 架构

```
客户端 (WebSocket)
    ↓
  [wss.go] 连接管理器 + 心跳 + 认证
    ↓
  [chat_handler.go] 聊天处理器 + 技能匹配 + 流式响应
    ↓
  [skill_executor.go] 技能注册中心 + LLM 执行
    ↓
  [llm.go] AWS Bedrock API
    ↓
  [s3_skill_loader.go] S3 技能加载
    ↓
  [events/] 事件存储 + 投影
```

## 监控建议

需要重点监控的指标：
- 活跃连接数
- 连接流失率
- 心跳延迟
- LLM 响应时间
- 单会话 Token 用量
- 认证失败次数
- Origin 拒绝率
