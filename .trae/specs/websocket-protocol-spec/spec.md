# WebSocket 协议规范

## Why

LingFlow 使用 WebSocket 实现客户端与服务端的实时双向通信，是整个系统的核心通信层。目前代码中已实现了基本的消息类型和通信流程，但缺乏统一的协议规范文档，导致：

1. 客户端接入时难以理解完整的消息格式和交互流程
2. 消息类型的扩展缺少明确的约束和约定
3. 错误处理和异常场景的处理方式不统一
4. 心跳和连接管理的细节缺少明确说明
5. 第三方开发者集成时缺少可参考的标准文档

## What Changes

本 Spec 定义 LingFlow WebSocket 通信协议的完整规范，包括：

- 连接建立与认证流程
- 消息信封格式与类型系统
- 各类消息的详细数据结构
- 完整的交互时序
- 心跳与连接管理机制
- 错误处理与重连策略
- 流式响应协议

### Impact

- Affected specs: 安全认证规范、事件溯源架构
- Affected code:
  - `internal/connections/wss.go` — WebSocket 连接管理
  - `internal/models/message.go` — 消息数据模型
  - `internal/models/responses.go` — 响应数据模型
  - `internal/services/chat_handler.go` — 消息处理
  - `demo/src/types/index.ts` — 前端类型定义

## ADDED Requirements

### Requirement: 连接建立与认证

系统 SHALL 定义标准的 WebSocket 连接建立流程。

#### 连接地址

```
ws://host:port/ws?token=<token>
wss://host:port/ws?token=<token>
```

#### 查询参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `token` | string | 是 | 认证 Token |
| `user_id` | string | 否 | 用户标识（Debug模式下可用） |

#### 连接流程

1. 客户端通过 REST API `/api/auth/token` 获取 Token
2. 客户端使用 Token 作为查询参数建立 WebSocket 连接
3. 服务端验证 Token 有效性
4. 验证通过则返回 101 Switching Protocols
5. 验证失败则返回 401 Unauthorized

#### Scenario: 成功建立连接

- **GIVEN** 客户端持有有效的认证 Token
- **WHEN** 发起 WebSocket 连接请求
- **THEN** 服务端返回 101，连接建立成功，服务端推送 `system_skills_list` 消息

#### Scenario: Token 无效

- **GIVEN** 客户端提供的 Token 无效或已过期
- **WHEN** 发起 WebSocket 连接请求
- **THEN** 服务端返回 401 状态码，连接被拒绝

---

### Requirement: 消息信封格式

系统 SHALL 使用统一的消息信封格式。

#### 消息结构

```json
{
    "type": "user_chat",
    "data": {},
    "skills_id": "/vulnerability_scanner",
    "timestamp": "2024-01-01T00:00:00Z"
}
```

#### 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 消息类型，见消息类型枚举 |
| `data` | object | 是 | 消息数据，结构随 type 变化 |
| `skills_id` | string | 否 | 关联的技能 ID |
| `timestamp` | string | 是 | ISO 8601 格式的时间戳 |

---

### Requirement: 消息类型系统

系统 SHALL 定义以下标准消息类型。

#### 客户端 → 服务端

| 类型 | 说明 |
|------|------|
| `user_chat` | 用户聊天消息 |
| `heartbeat_chat` | 心跳 Ping |

#### 服务端 → 客户端

| 类型 | 说明 |
|------|------|
| `system_chat` | 系统通知消息 |
| `system_thinking` | 思考过程消息（流式） |
| `system_response` | 最终响应消息（流式/完整） |
| `system_skills_list` | 技能列表推送 |
| `heartbeat_chat` | 心跳 Pong |

---

### Requirement: 用户聊天消息 (user_chat)

客户端发送用户聊天消息时 SHALL 使用以下结构。

#### Data 结构

```json
{
    "id": 1,
    "user_id": "user-123",
    "message": "检测系统安全漏洞",
    "selected_skill": "/vulnerability_scanner"
}
```

#### 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | number | 是 | 消息序列号（客户端维护） |
| `user_id` | string | 是 | 用户唯一标识 |
| `message` | string | 是 | 用户消息内容 |
| `selected_skill` | string | 否 | 用户选择的技能 ID，省略则自动匹配 |

#### 特殊命令

消息内容中支持以下特殊命令前缀：

- `#create_skill <name> <description>` — 创建新技能

#### Scenario: 发送普通聊天消息

- **GIVEN** WebSocket 连接已建立
- **WHEN** 客户端发送 `user_chat` 消息
- **THEN** 服务端开始处理，推送 `system_thinking` 和 `system_response`

---

### Requirement: 系统通知消息 (system_chat)

服务端推送系统通知时 SHALL 使用以下结构。

#### Data 结构

```json
{
    "event": "skills_updated",
    "message": "技能列表已更新",
    "skills": [
        {
            "id": "/vulnerability_scanner",
            "name": "漏洞扫描器",
            "description": "分析系统安全漏洞"
        }
    ]
}
```

#### 事件类型 (event)

| 事件 | 说明 |
|------|------|
| `skills_list` | 初始技能列表 |
| `skills_updated` | 技能列表更新 |
| `skill_created` | 技能创建成功 |
| `skill_create_failed` | 技能创建失败 |
| `connection_established` | 连接已建立 |
| `rate_limited` | 触发速率限制 |
| `invalid_message` | 消息格式无效 |
| `error` | 通用错误 |

#### Scenario: 连接建立后推送技能列表

- **GIVEN** WebSocket 连接刚建立成功
- **WHEN** 服务端完成技能加载
- **THEN** 推送 `system_chat` 消息，event 为 `skills_list`，包含所有可用技能

---

### Requirement: 思考过程消息 (system_thinking)

服务端在处理请求过程中 SHALL 推送思考过程消息。

#### Data 结构

```json
{
    "phase": "skill_selection",
    "content": "正在匹配最佳技能...",
    "matched_skills": [
        {
            "id": "/vulnerability_scanner",
            "score": 0.95
        }
    ]
}
```

#### 阶段 (phase)

| 阶段 | 说明 |
|------|------|
| `skill_selection` | 技能选择阶段 |
| `context_preparation` | 上下文准备阶段 |
| `llm_generation` | LLM 生成阶段 |
| `response_formatting` | 响应格式化阶段 |

#### Scenario: 流式思考过程

- **GIVEN** 服务端正在处理用户消息
- **WHEN** 进入不同处理阶段
- **THEN** 推送多条 `system_thinking` 消息，每条对应一个阶段

---

### Requirement: 系统响应消息 (system_response)

服务端返回最终响应时 SHALL 使用以下结构。

#### Data 结构（完整响应）

```json
{
    "id": "resp-abc123",
    "content": "根据分析，系统存在以下漏洞...",
    "tokens": {
        "input": 256,
        "output": 1024,
        "total": 1280
    },
    "latency_ms": 3500,
    "skill_id": "/vulnerability_scanner",
    "stop_reason": "end_turn"
}
```

#### Data 结构（流式响应片段）

```json
{
    "id": "resp-abc123",
    "content": "根据分析，",
    "is_final": false,
    "chunk_index": 1
}
```

#### 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 响应唯一标识 |
| `content` | string | 是 | 响应内容（完整或片段） |
| `tokens` | object | 否 | Token 用量（完整响应时提供） |
| `latency_ms` | number | 否 | 总延迟毫秒数 |
| `skill_id` | string | 否 | 使用的技能 ID |
| `stop_reason` | string | 否 | 停止原因 |
| `is_final` | boolean | 否 | 是否为最后一个片段 |
| `chunk_index` | number | 否 | 流片段序号 |

#### Scenario: 流式响应完成

- **GIVEN** 服务端正在流式返回响应
- **WHEN** 所有内容发送完毕
- **THEN** 发送最后一个 `system_response` 消息，`is_final=true`，并包含完整的统计信息

---

### Requirement: 心跳机制

系统 SHALL 实现 WebSocket 心跳检测机制。

#### 心跳消息

客户端和服务端均使用 `heartbeat_chat` 类型进行心跳。

#### Ping 消息（客户端 → 服务端）

```json
{
    "type": "heartbeat_chat",
    "data": {
        "event": "ping",
        "seq": 1
    },
    "skills_id": "",
    "timestamp": "2024-01-01T00:00:00Z"
}
```

#### Pong 消息（服务端 → 客户端）

```json
{
    "type": "heartbeat_chat",
    "data": {
        "event": "pong",
        "seq": 1
    },
    "skills_id": "",
    "timestamp": "2024-01-01T00:00:00Z"
}
```

#### 心跳参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| 心跳间隔 | 30秒 | 客户端发送 Ping 的间隔 |
| 心跳超时 | 60秒 | 服务端未收到 Ping 则断开连接 |
| 写入超时 | 10秒 | 消息写入超时时间 |

#### Scenario: 正常心跳

- **GIVEN** 连接已建立
- **WHEN** 客户端每 30 秒发送一次 Ping
- **THEN** 服务端每次收到 Ping 后立即回复 Pong，连接保持活跃

#### Scenario: 心跳超时断开

- **GIVEN** 客户端异常退出
- **WHEN** 服务端超过 60 秒未收到任何消息
- **THEN** 服务端主动关闭连接，清理资源

---

### Requirement: 错误处理

系统 SHALL 定义标准的错误消息格式。

#### 错误消息结构

```json
{
    "type": "system_chat",
    "data": {
        "event": "error",
        "error_code": "INVALID_MESSAGE_FORMAT",
        "message": "消息格式无效",
        "details": {}
    },
    "skills_id": "",
    "timestamp": "2024-01-01T00:00:00Z"
}
```

#### 错误码

| 错误码 | 说明 | HTTP 状态 |
|--------|------|-----------|
| `INVALID_TOKEN` | 认证 Token 无效 | 401 |
| `CONNECTION_LIMIT_EXCEEDED` | 连接数超出限制 | 429 |
| `RATE_LIMITED` | 请求速率超限 | 429 |
| `INVALID_MESSAGE_FORMAT` | 消息格式无效 | - |
| `UNSUPPORTED_MESSAGE_TYPE` | 不支持的消息类型 | - |
| `SKILL_NOT_FOUND` | 技能不存在 | - |
| `SKILL_CREATE_DISABLED` | 技能创建功能未启用 | 403 |
| `INVALID_SKILL_NAME` | 技能名称非法 | - |
| `SKILL_ALREADY_EXISTS` | 技能已存在 | - |
| `INTERNAL_ERROR` | 内部错误 | 500 |

#### 生产环境错误处理

- 生产模式下，错误消息不得暴露内部实现细节
- `message` 字段使用通用描述
- `details` 字段在生产环境中不返回

#### Scenario: 生产环境隐藏错误细节

- **GIVEN** `MODE=production`
- **WHEN** 发生内部错误
- **THEN** 返回错误码 `INTERNAL_ERROR`，消息为 "服务暂时不可用，请稍后重试"，不包含 details

---

### Requirement: 技能列表推送

系统 SHALL 在连接建立和技能更新时推送技能列表。

#### 消息结构

```json
{
    "type": "system_chat",
    "data": {
        "event": "skills_list",
        "skills": [
            {
                "id": "/vulnerability_scanner",
                "name": "漏洞扫描器",
                "display_name": "漏洞扫描分析",
                "description": "分析系统安全漏洞，提供漏洞评估和修复建议",
                "category": "security",
                "tags": ["漏洞扫描", "安全评估"]
            }
        ]
    },
    "skills_id": "",
    "timestamp": "2024-01-01T00:00:00Z"
}
```

#### 推送时机

1. 连接建立成功后立即推送
2. 创建新技能成功后推送给所有连接

---

### Requirement: 完整交互时序

系统 SHALL 遵循以下标准交互流程。

#### 正常聊天流程

```
客户端                          服务端                          AWS
  │                               │                              │
  │  1. POST /api/auth/token      │                              │
  │  { "user_id": "xxx" }         │                              │
  │──────────────────────────────►│                              │
  │                               │ 验证用户身份                 │
  │  2. { token, user_id }        │                              │
  │◄──────────────────────────────│                              │
  │                               │                              │
  │  3. WS Connect ?token=xxx     │                              │
  │──────────────────────────────►│                              │
  │                               │ 验证 Token                   │
  │  4. 101 Switching Protocols   │                              │
  │◄──────────────────────────────│                              │
  │                               │                              │
  │  5. system_skills_list        │  LoadAllSkills()             │
  │  (技能列表推送)                │◄─────────────────────────────│ S3
  │◄──────────────────────────────│                              │
  │                               │                              │
  │  6. user_chat                 │                              │
  │  { message: "分析漏洞" }       │                              │
  │──────────────────────────────►│                              │
  │                               │ 技能匹配                     │
  │  7. system_thinking           │                              │
  │  { phase: skill_selection }   │                              │
  │◄──────────────────────────────│                              │
  │                               │                              │
  │                               │  Converse API               │
  │                               │─────────────────────────────►│ Bedrock
  │                               │  生成响应                     │
  │  8. system_thinking           │                              │
  │  { phase: llm_generation }    │                              │
  │◄──────────────────────────────│                              │
  │                               │                              │
  │  9. system_response           │                              │
  │  { content, tokens, latency } │                              │
  │◄──────────────────────────────│                              │
  │                               │                              │
  │  10. heartbeat_chat (ping)    │                              │
  │──────────────────────────────►│                              │
  │  11. heartbeat_chat (pong)    │                              │
  │◄──────────────────────────────│                              │
```

---

### Requirement: 重连策略

客户端 SHALL 实现 WebSocket 重连机制。

#### 重连策略

- 初始重连延迟：1秒
- 指数退避：每次失败延迟翻倍
- 最大延迟：30秒
- 最大重试次数：无限制（或可配置）
- 重连成功后重置延迟
- 重连时重新获取 Token（如果已过期）

#### 重连后状态恢复

- 重新获取技能列表
- 保留本地消息历史
- 恢复未完成的流式显示（可选）
