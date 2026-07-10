# LingFlow 代码风格指南

## Go 代码风格

### 命名约定

#### 包名
- 使用小写，简短且有描述性
- 不使用下划线或混合大小写
- 包名应与目录名一致

```go
// 正确
package services
package events
package utilities

// 错误
package service_layer
package eventSourcing
```

#### 结构体名
- 使用 PascalCase
- 首字母大写表示导出
- 名称应为名词或名词短语

```go
type ChatHandler struct {
    // ...
}

type WebSocketConnectionManager struct {
    // ...
}
```

#### 接口名
- 使用 PascalCase
- 单个方法的接口以 -er 结尾
- 多个方法的接口使用描述性名称

```go
type EventStore interface {
    // ...
}

type Notifier interface {
    Notify(ctx context.Context, message string) error
}
```

#### 函数与方法
- 使用 PascalCase（导出）或 camelCase（私有）
- 方法名应为动词或动词短语
- Getter 方法不使用 Get 前缀

```go
// 导出方法
func (h *ChatHandler) HandleIncomingMessage(ctx context.Context, msg models.WSMessage) error {
    // ...
}

// 私有方法
func (h *ChatHandler) processSkillSelection(msg string) string {
    // ...
}

// Getter 方法
func (s *SkillRegistry) Skills() []SkillInfo {
    // ...
}
```

#### 常量
- 使用 PascalCase
- 相关常量分组声明
- 使用 iota 定义枚举

```go
type MessageType string

const (
    TypeUserChat      MessageType = "user_chat"
    TypeSystemChat    MessageType = "system_chat"
    TypeSystemThinking MessageType = "system_thinking"
    TypeSystemResponse MessageType = "system_response"
    TypeHeartbeat     MessageType = "heartbeat_chat"
)
```

#### 变量
- 使用 camelCase
- 变量名应体现其用途
- 短变量名用于短作用域

```go
// 局部变量
conn, ok := m.connections[connID]

// 包级变量
var logger = utilities.NewLogger()
```

---

### 代码组织

#### 文件结构
- 每个文件关注一个主要类型或功能组
- 相关类型和方法可以放在同一文件
- 大文件应拆分为多个小文件

```
internal/services/
    chat_handler.go       - ChatHandler 主逻辑
    skill_executor.go     - 技能执行逻辑
    skill_registry.go     - 技能注册表
    skill_creator.go      - 技能创建逻辑
    llm.go                - LLM 服务接口和实现
    s3_skill_loader.go    - S3 技能加载
    auth_handler.go       - 认证处理
    server.go             - 服务组装
    notifier.go           - 通知接口
```

#### 导入顺序
- 标准库
- 第三方库
- 内部包
- 每组之间空行分隔

```go
import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/gorilla/websocket"
    "github.com/aws/aws-sdk-go-v2/service/s3"

    "lingflow/internal/models"
    "lingflow/internal/utilities"
)
```

---

### 错误处理

#### 错误返回
- 函数返回最后一个值为 error
- 错误信息使用中文
- 错误信息应描述问题，不包含调用者信息

```go
func (s *S3SkillLoader) LoadSkill(ctx context.Context, skillID string) (*models.Skill, error) {
    // ...
    if err != nil {
        return nil, fmt.Errorf("加载技能失败: %w", err)
    }
    return skill, nil
}
```

#### 错误包装
- 使用 `fmt.Errorf("%w", err)` 包装错误
- 保留原始错误链
- 添加上下文信息

```go
result, err := s.client.GetObject(ctx, input)
if err != nil {
    return nil, fmt.Errorf("S3 获取对象失败: %w", err)
}
```

#### 错误日志
- 记录错误时包含足够的上下文
- 使用统一的 Logger
- 日志使用中文

```go
s.logger.Error("技能创建失败", map[string]interface{}{
    "skill_id": skillID,
    "error":    err.Error(),
})
```

---

### 注释

#### 包注释
- 每个包的第一个文件应有包注释
- 描述包的用途和主要功能

```go
// Package services 提供 LingFlow 核心业务逻辑服务，
// 包括聊天处理、技能执行、LLM 调用、S3 技能管理等功能。
package services
```

#### 类型注释
- 导出类型必须有注释
- 描述类型的用途和职责

```go
// ChatHandler 处理 WebSocket 消息的核心业务逻辑，
// 负责消息路由、技能匹配和响应生成。
type ChatHandler struct {
    // ...
}
```

#### 函数注释
- 导出函数必须有注释
- 描述函数做什么，而非怎么做
- 说明参数和返回值的含义

```go
// HandleIncomingMessage 处理客户端发送的 WebSocket 消息，
// 根据消息类型分发到不同的处理逻辑。
//
// 参数:
//   ctx - 上下文，用于控制取消和超时
//   msg - 客户端消息
//
// 返回:
//   error - 处理过程中的错误
func (h *ChatHandler) HandleIncomingMessage(ctx context.Context, msg models.WSMessage) error {
    // ...
}
```

---

### 上下文使用

- 第一个参数通常是 `context.Context`
- 传递上下文而不是存储它
- 使用 context 控制超时和取消

```go
func (s *LLMService) GenerateResponse(ctx context.Context, prompt string) (string, error) {
    ctx, cancel := context.WithTimeout(ctx, s.timeout)
    defer cancel()
    // ...
}
```

---

## TypeScript / Vue 代码风格

### 组件命名

- 组件文件名使用 PascalCase
- 组件名使用多词，避免单个词
- 基础组件使用 Base 前缀

```
components/
    ChatHeader.vue
    ChatInput.vue
    MessageBubble.vue
    MessagesList.vue
    SkillsPanel.vue
    AuthModal.vue
```

### Props 定义

- 使用 `defineProps` 类型声明
- 必填 props 不使用 ?
- 可选 props 使用 ? 或提供默认值

```typescript
const props = defineProps<{
    message: ChatMessage
    isLoading?: boolean
}>()
```

### 组合式函数

- 文件名使用 camelCase
- 以 `use` 开头
- 返回响应式状态和方法

```typescript
// useChatStore.ts
export function useChatStore() {
    // ...
    return {
        messages,
        isConnected,
        sendMessage,
        connect,
        disconnect
    }
}
```

### 类型定义

- 集中在 `types/` 目录
- 使用 interface 定义对象形状
- 使用 type 定义联合类型和工具类型

```typescript
// types/index.ts
export interface ChatMessage {
    id: string
    type: MessageType
    content: string
    timestamp: Date
    skillsId?: string
}

export type MessageType = 'user' | 'thinking' | 'response' | 'system'
```

### 模板

- 指令使用完整形式
- 属性使用 kebab-case
- 事件使用 kebab-case

```vue
<template>
    <div class="message-bubble" :class="{ 'is-user': isUser }">
        <div class="message-content">
            <slot></slot>
        </div>
    </div>
</template>
```

---

## CSS 样式规范

### 命名约定

- 使用 BEM 风格或组件化命名
- 类名使用 kebab-case
- 状态使用 is- 前缀

```css
.chat-input {
    /* ... */
}

.chat-input__textarea {
    /* ... */
}

.chat-input--focused {
    /* ... */
}

.chat-input.is-disabled {
    /* ... */
}
```

### CSS 变量

- 全局变量定义在根选择器
- 使用 -- 前缀
- 语义化命名

```css
:root {
    --bg-deep: #0a0a0f;
    --bg-surface: #12121a;
    --accent: #ff6b9d;
    --text-primary: #f5f5f7;
    --text-secondary: #a1a1aa;
    --spacing-sm: 8px;
    --spacing-md: 16px;
    --spacing-lg: 24px;
    --radius-sm: 8px;
    --radius-md: 12px;
    --radius-lg: 16px;
}
```

---

## Markdown 文档规范

### 标题层级

- 一级标题：文档标题（每个文档仅一个）
- 二级标题：主要章节
- 三级标题：子章节
- 四级及以下：细节小节

```markdown
# 文档标题

## 章节一

### 子章节 1.1

#### 细节点
```

### 代码块

- 指定语言
- Go、TypeScript、JavaScript、bash、yaml、json 等

```go
func main() {
    fmt.Println("Hello, World!")
}
```

### 列表

- 使用 - 或 * 作为无序列表标记
- 使用 1. 作为有序列表标记
- 嵌套使用缩进

```markdown
- 项目一
  - 子项目 A
  - 子项目 B
- 项目二
- 项目三

1. 第一步
2. 第二步
3. 第三步
```

### 链接

- 使用描述性文本
- 内部链接使用相对路径
- 外部链接使用完整 URL

```markdown
[README](README.md)
[AWS SDK](https://aws.amazon.com/sdk-for-go/)
```

### 图片

- 使用相对路径
- 添加 alt 文本
- 重要图片在文档中说明

```markdown
![架构图](docs/images/architecture.png)
```
