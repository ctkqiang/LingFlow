# LingFlow Go 代码注释规范

## 概述

本文档定义 LingFlow 项目中 Go 代码的注释编写规范。所有贡献者在编写或修改 Go 代码时必须遵守本规范，以确保代码库的注释风格统一、文档生成质量可控、团队协作高效。

### 核心原则

1. **语言统一**：所有注释使用中文（简体），技术术语保留英文原文（如 `WebSocket`、`goroutine`、`context`）
2. **godoc 兼容**：所有导出符号的注释必须符合 `godoc` 生成规范
3. **句号规范**：godoc 注释使用中文句号 `。` 结尾；行尾短注释可省略句号
4. **注释即文档**：注释描述"做什么"和"为什么"，而非"怎么做"
5. **不使用 emoji**：注释中不使用任何 emoji 符号

---

## 1. 注释类型规范

### 1.1 包注释（Package Comments）

每个包的**第一个文件**（通常是 `doc.go` 或包名同名文件）必须包含包级注释。包注释位于 `package` 声明之前，描述包的整体职责、核心功能和使用场景。

#### 格式要求

- 以 `// Package <包名>` 开头
- 第一段为简短的功能概述（一到两句话）
- 后续段落可补充使用场景、设计决策、注意事项等
- 段落之间使用空注释行 `//` 分隔

#### 正面示例

```go
// Package events 实现 LingFlow 的事件溯源核心机制。
//
// 本包提供领域事件的定义、存储、发布和投影能力，
// 是系统状态管理和业务逻辑追溯的基础设施层。
//
// 核心组件：
//   - DomainEvent    : 不可变的事实记录
//   - EventStore     : 事件持久化接口
//   - EventBus       : 事件订阅与发布
//   - Aggregate      : 聚合根，封装业务规则
//   - Projection     : 通过事件增量构建查询状态
//
// 使用场景：
//   - 所有业务操作通过 Command 发起，由 Aggregate 验证后产生 Event
//   - EventStore 持久化所有事件，EventBus 负责事件通知
//   - Projection 订阅事件并构建读模型
package events
```

#### 反面示例

```go
// 这个包是事件相关的。
package events
```

**问题**：未以 `Package events` 开头，描述过于笼统，不符合 godoc 格式。

```go
/*
 * events 包
 * 处理事件
 */
package events
```

**问题**：使用了块注释 `/* */`，Go 社区约定包注释使用行注释 `//`。

---

### 1.2 函数/方法注释（Function/Method Comments）

所有**导出函数和方法**必须有注释。未导出函数在逻辑复杂或不直观时也应添加注释。

#### 格式要求

- 以函数名开头：`// FuncName 中文描述。`
- 第一句话为功能概述
- 复杂函数使用多段注释，段落间用空注释行分隔
- 参数说明使用 godoc 列表格式
- 返回值说明使用 godoc 列表格式
- 错误处理说明在返回值部分或单独段落中描述

#### 简单函数 -- 正面示例

```go
// NewDomainEvent 根据事件类型和数据构建领域事件。
func NewDomainEvent(
    eventType EventType,
    streamID string,
    aggregateID string,
    version int,
    data interface{},
) (DomainEvent, error) {
    // ...
}
```

#### 复杂函数 -- 正面示例

```go
// Execute 通过完整管道处理用户消息：
//  1. 为用户查询检索最佳匹配技能
//  2. 为 LLM 构建技能增强上下文
//  3. 通过 LLM 服务生成响应
//  4. 将响应格式化为 WSMessage
//
// 参数：
//   - ctx       : 上下文，用于控制取消和超时
//   - userQuery : 用户输入的原始文本
//   - connID    : 连接标识符，用于日志追踪
//
// 返回：
//   - ExecutionResult : 包含生成的消息和匹配的技能信息
//   - error           : 任一阶段失败时返回包装后的错误
func (executor *SkillExecutor) Execute(
    ctx context.Context,
    userQuery string,
    connID string,
) (ExecutionResult, error) {
    // ...
}
```

#### 带使用示例的注释

```go
// Reload 清空并替换技能注册中心的内容。
// 通常在 S3 中新增/修改技能后调用，以同步内存索引。
// 与逐个 RegisterSkill 相比，Reload 是原子操作，期间不会留下空状态。
//
// 参数：
//   - skills : 新的完整技能列表（通常来自 S3SkillLoader.LoadAllSkills）
//
// 返回：任一技能缺少必填字段时返回错误；成功时返回 nil。
//
// 使用示例：
//
//	allSkills, err := loader.LoadAllSkills(ctx)
//	if err != nil {
//	    return fmt.Errorf("加载技能失败: %w", err)
//	}
//	if err := registry.Reload(allSkills); err != nil {
//	    return fmt.Errorf("重载技能注册中心失败: %w", err)
//	}
func (registry *SkillRegistry) Reload(skills []models.SkillDefinition) error {
    // ...
}
```

#### 反面示例

```go
// 处理消息
func (h *ChatHandler) HandleIncomingMessage(ctx context.Context, msg models.WSMessage) error {
```

**问题**：未以函数名开头，描述过于简略，缺少参数和返回值说明。

```go
// HandleIncomingMessage
// 这个函数处理传入的消息
// 参数: ctx, msg
// 返回: error
func (h *ChatHandler) HandleIncomingMessage(ctx context.Context, msg models.WSMessage) error {
```

**问题**：第一行只有函数名没有描述，参数说明过于简略，未说明具体含义。

---

### 1.3 类型注释（Type Comments）

所有**导出类型**（结构体、接口、类型别名、枚举）必须有注释。

#### 结构体注释

- 以类型名开头：`// TypeName 中文描述。`
- 复杂结构体使用多段注释解释设计意图和字段语义
- 结构体字段使用行尾注释

##### 正面示例

```go
// DomainEvent 是事件溯源中唯一持久化的事实记录。
//
// StreamID 用于定位一条事件流，例如 "chat:<uuid>"。
// AggregateID 用于标识业务对象，例如 chat uuid。
// Version 是同一 StreamID 内的递增版本号。
type DomainEvent struct {
    EventID     string      // 全局唯一事件标识符
    EventType   EventType   // 事件类型
    StreamID    string      // 事件流标识符
    AggregateID string      // 聚合根标识符
    Version     int         // 流内递增版本号
    OccurredAt  time.Time   // 事件发生时间
    Data        interface{} // 事件携带的业务数据
}
```

##### 反面示例

```go
// 事件结构体
type DomainEvent struct {
    EventID     string
    EventType   EventType
    StreamID    string
    AggregateID string
    Version     int
    OccurredAt  time.Time
    Data        interface{}
}
```

**问题**：未以类型名开头，字段缺少注释，描述过于笼统。

#### 接口注释

- 以接口名开头
- 描述接口定义的抽象能力
- 接口方法也应有注释

##### 正面示例

```go
// EventStore 定义事件追加与读取能力。
type EventStore interface {
    // Append 将事件追加到指定的事件流中。
    Append(ctx context.Context, events []DomainEvent) error
    // ReadStream 读取指定事件流从 afterVersion 之后的所有事件。
    ReadStream(ctx context.Context, streamID string, afterVersion int) ([]DomainEvent, error)
}
```

##### 反面示例

```go
type EventStore interface {
    Append(ctx context.Context, events []DomainEvent) error
    ReadStream(ctx context.Context, streamID string, afterVersion int) ([]DomainEvent, error)
}
```

**问题**：接口本身和方法均缺少注释。

#### 类型别名和枚举注释

```go
// EventType 表示系统中已经发生的事实类型。
type EventType string

// MessageType 定义 WebSocket 消息的类型标识。
type MessageType string
```

---

### 1.4 变量注释（Variable Comments）

导出变量必须有注释，描述变量的用途和行为。未导出的包级变量在用途不明显时也应添加注释。

#### 正面示例

```go
// CloudWatchMode 为 true 时，所有日志函数在输出人类可读格式的同时，
// 也会向 stderr 输出一行 CloudWatch 兼容的 JSON 结构化日志。
var CloudWatchMode bool
```

#### 错误变量组

错误变量组使用 `var` 块声明，每个错误变量的错误消息本身即为文档：

```go
var (
    ErrSkillBucketNameEmpty = errors.New("技能: bucket 名称为空")
    ErrSkillObjectKeyEmpty  = errors.New("技能: 对象键为空")
    ErrSkillIdentifierEmpty = errors.New("技能: 标识符为空")
)
```

当错误消息不足以说明用途时，添加行尾注释：

```go
var (
    ErrConcurrencyConflict = errors.New("并发冲突") // 乐观锁版本不匹配时返回
    ErrStreamNotFound      = errors.New("事件流不存在")
)
```

---

### 1.5 常量注释（Constant Comments）

#### 枚举常量组

使用行尾注释，与值对齐：

```go
const (
    UserChat         MessageType = "user_chat"          // 用户发送的普通聊天消息
    SystemChat       MessageType = "system_chat"        // 系统发送的通知类消息
    SystemThinking   MessageType = "system_thinking"    // 系统思考过程（技能匹配、推理）
    SystemResponse   MessageType = "system_response"    // 系统最终响应消息
    SystemSkillsList MessageType = "system_skills_list" // 服务端推送的可用技能列表
    HeartbeatChat    MessageType = "heartbeat_chat"     // 心跳消息（ping/pong）
)
```

#### iota 枚举常量组

```go
const (
    DEBUG   LogLevel = iota // 详细诊断信息（仅限本地开发）
    INFO                    // 通用运行消息（默认级别）
    WARN                    // 可恢复的问题，降级模式
    ERROR                   // 需要关注的故障
    VERBOSE                 // 逐请求指标，审计追踪
)
```

#### 配置常量

技术配置常量可省略注释，但非直观的值应添加说明：

```go
const (
    defaultHeartbeatInterval     = 30 * time.Second
    defaultHeartbeatTimeout      = 90 * time.Second
    defaultHeartbeatWriteTimeout = 10 * time.Second
    defaultMaxConnectionsPerIP   = 10
    defaultMaxFrameBytes         = 64 * 1024 // 64KB WebSocket frame cap
)
```

#### 独立常量

```go
// configLoaderComponent 是本文件在日志中使用的组件名称标识。
const configLoaderComponent = "ConfigLoader"

// secretsManagerTimeout 是单次 Secrets Manager 或 S3 API 调用的超时时间。
const secretsManagerTimeout = 10 * time.Second
```

---

## 2. 注释语法规范

### 2.1 单行注释（`//`）

单行注释是 LingFlow 项目中**唯一推荐**的注释语法。

#### 适用场景

| 场景 | 示例 |
|------|------|
| godoc 文档注释 | `// NewDomainEvent 根据事件类型和数据构建领域事件。` |
| 代码行解释 | `// 获取该IP的最新连接数` |
| 步骤分段标记 | `// ── 步骤1: TLS 检查 ──` |
| 校验步骤标记 | `// 校验步骤1: 类型非空` |
| TODO 标记 | `// TODO: 用户需要实现的认证逻辑` |
| 分隔线 | `// ---------------------------------------------------------------------------` |

#### 格式规则

- `//` 后面必须有一个空格：`// 正确` 而非 `//错误`
- godoc 注释紧贴被注释的声明，中间不留空行
- 多段 godoc 注释使用空注释行 `//` 分隔段落
- 行尾注释与代码之间至少一个空格

### 2.2 块注释（`/* */`）

**原则上不使用块注释。** Go 社区约定和本项目实践均使用 `//` 行注释。

#### 唯一允许的例外

- 临时注释掉大段代码用于调试（调试完成后必须删除）
- 自动生成的版权声明头（如果有）

#### 禁止使用块注释的场景

- godoc 文档注释
- 包注释
- 函数/类型注释
- 任何将保留在代码库中的注释

### 2.3 缩进与格式规则

#### godoc 列表格式

使用两个空格缩进 + 短横线：

```go
// 参数：
//   - ctx       : 上下文，用于控制取消和超时
//   - userQuery : 用户输入的原始文本
```

#### godoc 编号列表

使用缩进 + 数字：

```go
// Execute 通过完整管道处理用户消息：
//  1. 为用户查询检索最佳匹配技能
//  2. 为 LLM 构建技能增强上下文
//  3. 通过 LLM 服务生成响应
```

#### godoc 代码块

使用一个 Tab 或四个空格缩进：

```go
// 使用示例：
//
//	allSkills, err := loader.LoadAllSkills(ctx)
//	if err != nil {
//	    return fmt.Errorf("加载技能失败: %w", err)
//	}
```

#### 步骤分段标记

使用 `── 步骤N:` 或 `── 阶段N:` 格式：

```go
// ── 步骤1: TLS 检查 ──
if config.CertFile != "" && config.KeyFile != "" {
    // ...
}

// ── 步骤2: 创建 HTTP 多路复用器 ──
mux := http.NewServeMux()
```

#### 分隔线

用于分隔文件内的逻辑区域：

```go
// ---------------------------------------------------------------------------
// CloudWatch JSON 结构化日志
// ---------------------------------------------------------------------------
```

### 2.4 标点符号规范

| 注释类型 | 句末标点 | 示例 |
|----------|----------|------|
| godoc 文档注释 | 中文句号 `。` | `// NewEvent 创建新的领域事件。` |
| 多段注释的每段末尾 | 中文句号 `。` | `// 发布时先以读锁获取处理器快照。` |
| 行尾短注释 | 可省略 | `Region string // AWS 区域` |
| 枚举常量行尾注释 | 可省略 | `DEBUG LogLevel = iota // 详细诊断信息` |
| 步骤标记 | 不使用句号 | `// ── 步骤1: TLS 检查 ──` |

---

## 3. Godoc 文档生成规范

### 3.1 基本规则

Go 的 `godoc` 工具从源代码注释自动生成 API 文档。要生成高质量文档，注释必须遵循以下规则：

1. **紧贴声明**：注释必须紧贴在被注释的声明之前，中间不能有空行
2. **以名称开头**：注释的第一个词必须是被注释的符号名称
3. **完整句子**：第一句话应是完整的功能概述

```go
// 正确：注释紧贴声明
// RegisterSkill 在注册中心中添加或更新一个技能。
func (registry *SkillRegistry) RegisterSkill(...) error {

// 错误：注释与声明之间有空行（godoc 不会关联）
// RegisterSkill 在注册中心中添加或更新一个技能。

func (registry *SkillRegistry) RegisterSkill(...) error {
```

### 3.2 段落分隔

使用空注释行分隔段落：

```go
// Publish 将事件发布到所有已订阅的处理器。
//
// 对每个事件，先以读锁获取对应事件类型的处理器快照，
// 释放锁后再逐个调用处理器。如果任一处理器返回错误，
// 则立即返回该错误；所有处理器均成功时返回 nil。
func (bus *InMemoryEventBus) Publish(...) error {
```

在 godoc 输出中，空注释行会被渲染为段落分隔。

### 3.3 标题

Go 1.19+ 的 godoc 支持在注释中使用标题。标题行必须满足：

- 独占一行
- 以 `#` 开头（Go 1.19+ 风格），或者
- 不以小写字母开头，不以标点结尾，后跟空注释行

```go
// Package connections 管理 WebSocket 连接的生命周期。
//
// # 连接管理
//
// 本包提供连接池管理、心跳检测、消息路由等核心功能。
//
// # 安全机制
//
// 支持 Origin 校验、IP 限流、TLS 终止等安全特性。
package connections
```

### 3.4 代码块

在注释中嵌入代码示例时，使用一个 Tab 缩进（相对于 `//` 后的文本）：

```go
// LoadConfig 根据运行时环境选择配置加载策略。
//
// 使用示例：
//
//	ctx := context.Background()
//	if err := LoadConfig(ctx); err != nil {
//	    log.Fatalf("配置加载失败: %v", err)
//	}
func LoadConfig(ctx context.Context) error {
```

### 3.5 列表

godoc 支持两种列表格式：

**无序列表**（使用 `- ` 或 `* ` 前缀）：

```go
// 核心组件：
//   - DomainEvent    : 不可变的事实记录
//   - EventStore     : 事件持久化接口
//   - EventBus       : 事件订阅与发布
```

**有序列表**（使用数字前缀）：

```go
// 处理流程：
//  1. 接收 WebSocket 消息
//  2. 解析消息类型
//  3. 分发到对应处理器
```

### 3.6 链接

godoc 会自动将 URL 渲染为可点击链接：

```go
// 更多信息请参阅 https://docs.aws.amazon.com/bedrock/latest/userguide/
```

引用同包或其他包的符号：

```go
// HandleIncomingMessage 使用 [SkillExecutor] 处理技能相关消息。
// 消息格式定义参见 [models.WSMessage]。
```

---

## 4. 示例代码规范

### 4.1 注释内示例

在 godoc 注释中嵌入示例代码时，使用缩进代码块：

```go
// Reload 清空并替换技能注册中心的内容。
//
// 使用示例：
//
//	allSkills, err := loader.LoadAllSkills(ctx)
//	if err != nil {
//	    return fmt.Errorf("加载技能失败: %w", err)
//	}
//	if err := registry.Reload(allSkills); err != nil {
//	    return fmt.Errorf("重载技能注册中心失败: %w", err)
//	}
func (registry *SkillRegistry) Reload(skills []models.SkillDefinition) error {
```

### 4.2 Example 函数

Example 函数是 Go 测试框架提供的可执行文档示例，位于 `_test.go` 文件中。

#### 命名规则

| 目标 | Example 函数名 | 说明 |
|------|----------------|------|
| 包级示例 | `Example()` | 展示包的整体用法 |
| 函数示例 | `ExampleFuncName()` | 展示函数用法 |
| 类型示例 | `ExampleTypeName()` | 展示类型用法 |
| 方法示例 | `ExampleTypeName_MethodName()` | 展示方法用法 |
| 多个示例 | `ExampleFuncName_suffix()` | suffix 为小写描述 |

#### 编写标准

```go
// 文件: internal/events/event_test.go

func ExampleNewDomainEvent() {
    event, err := events.NewDomainEvent(
        events.EventTypeSessionConnected,
        "chat:abc-123",
        "abc-123",
        1,
        map[string]string{"user_id": "user-001"},
    )
    if err != nil {
        fmt.Printf("创建事件失败: %v\n", err)
        return
    }
    fmt.Printf("事件类型: %s\n", event.EventType)
    fmt.Printf("事件流: %s\n", event.StreamID)
    // Output:
    // 事件类型: session.connected
    // 事件流: chat:abc-123
}

func ExampleSkillRegistry_Reload() {
    registry := services.NewSkillRegistry()
    skills := []models.SkillDefinition{
        {
            SkillIdentifier:  "greeting",
            SkillDisplayName: "问候技能",
            SkillDescription: "处理用户的问候和寒暄",
        },
    }
    if err := registry.Reload(skills); err != nil {
        fmt.Printf("重载失败: %v\n", err)
        return
    }
    fmt.Println("技能注册成功")
    // Output:
    // 技能注册成功
}
```

#### 最佳实践

1. **必须包含 `// Output:` 注释**：使 Example 函数成为可执行测试
2. **保持简洁**：只展示核心用法，不包含复杂的错误处理链
3. **使用有意义的数据**：避免 `foo`、`bar` 等无意义占位符
4. **独立可运行**：不依赖外部状态或环境变量

---

## 5. 内联注释规范

### 5.1 步骤分段标记

用于长函数中标记逻辑阶段，提高可读性：

```go
func (server *Server) Start(ctx context.Context) error {
    // ── 步骤1: TLS 检查 ──
    if config.CertFile != "" && config.KeyFile != "" {
        // ...
    }

    // ── 步骤2: 创建 HTTP 多路复用器 ──
    mux := http.NewServeMux()

    // ── 步骤3: 初始化事件存储 ──
    store := NewInMemoryEventStore()
}
```

### 5.2 校验步骤标记

用于校验逻辑中标记每个校验点：

```go
func validateEvent(event DomainEvent) error {
    // 校验步骤1: 类型非空
    if event.EventType == "" {
        return errors.New("事件类型不能为空")
    }

    // 校验步骤2: 类型合法性
    if !isValidEventType(event.EventType) {
        return fmt.Errorf("未知事件类型: %s", event.EventType)
    }

    // 校验步骤3: 数据非空
    if event.Data == nil {
        return errors.New("事件数据不能为空")
    }
}
```

### 5.3 解释性注释

用于解释非直观的代码逻辑：

```go
// 已存在的键不覆盖（Lambda IAM 注入的凭证优先）。
if _, exists := os.LookupEnv(key); !exists {
    os.Setenv(key, value)
}
```

### 5.4 TODO 注释

标记待完成的工作：

```go
// TODO: 用户需要实现的认证逻辑
// TODO: 添加 Redis 作为 EventStore 的持久化后端
```

---

## 6. 未导出符号注释规范

未导出（小写开头）的函数、类型、变量不会出现在 godoc 文档中，但在以下情况仍应添加注释：

### 6.1 需要注释的情况

- 逻辑复杂或不直观的函数
- 有并发安全要求的函数（如需要调用方持有锁）
- 包含重要业务规则的函数

```go
// rebuildMetadataIndexLocked 从当前技能重建元数据索引。
// 调用方必须持有写锁。
func (registry *SkillRegistry) rebuildMetadataIndexLocked() {
    // ...
}

// isExpectedCloseError 判断 WebSocket 关闭是否为正常原因
// （客户端断开、正常关闭握手、空闲超时）。
func isExpectedCloseError(err error) bool {
    // ...
}
```

### 6.2 可省略注释的情况

- 名称已充分表达意图的简单辅助函数
- 短小的工具函数（如 `min`、`max`、`contains`）

---

## 7. 规范检查工具

### 7.1 golangci-lint 配置

在项目根目录创建 `.golangci.yml`：

```yaml
# .golangci.yml
run:
  timeout: 5m
  go: "1.26.1"

linters:
  enable:
    # 注释规范检查
    - godot          # 检查注释是否以句号结尾
    - revive         # 包含多种注释规范检查规则
    - stylecheck     # 检查 Go 代码风格，包括注释格式

    # 代码质量
    - govet          # Go 官方静态分析
    - errcheck       # 检查未处理的错误
    - staticcheck    # 高级静态分析
    - gosimple       # 简化代码建议
    - unused         # 检查未使用的代码

linters-settings:
  godot:
    # 检查所有顶级注释（不仅是导出符号）
    scope: toplevel
    # 允许的句末字符（中文句号和英文句号）
    period: true
    # 允许中文句号
    capital: false

  revive:
    rules:
      # 导出符号必须有注释
      - name: exported
        severity: warning
        arguments:
          - "checkPrivateReceivers"
          - "sayRepetitiveInsteadOfStutters"
      # 包必须有注释
      - name: package-comments
        severity: warning

  stylecheck:
    # 检查导出符号注释是否以符号名开头
    dot-import-whitelist: []
    # ST1000: 包注释格式
    # ST1020: 导出函数注释格式
    # ST1021: 导出类型注释格式
    checks:
      - "all"
      - "-ST1003" # 允许非标准命名（中文项目可能有特殊需求）

issues:
  # 不跳过测试文件的注释检查
  exclude-use-default: false
  exclude-rules:
    # 测试文件中放宽注释要求
    - path: _test\.go
      linters:
        - godot
        - revive
```

### 7.2 本地运行

```bash
# 安装 golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# 运行全部检查
golangci-lint run ./...

# 仅运行注释相关检查
golangci-lint run --enable godot,revive,stylecheck --disable-all ./...

# 自动修复部分问题（如 godot 可自动添加句号）
golangci-lint run --fix ./...
```

### 7.3 CI/CD 集成

#### GitHub Actions

在 `.github/workflows/codeql.yml` 的 lint 阶段添加：

```yaml
go-lint:
  name: Go 代码规范检查
  runs-on: ubuntu-latest
  steps:
    - name: 检出仓库代码
      uses: actions/checkout@v4

    - name: 配置 Go 环境
      uses: actions/setup-go@v5
      with:
        go-version: ${{ env.GO_VERSION }}
        cache: true

    - name: 运行 golangci-lint
      uses: golangci/golangci-lint-action@v6
      with:
        version: latest
        args: --timeout=5m
```

#### GitLab CI

在 `.gitlab-ci.yml` 的 lint 阶段添加：

```yaml
go:lint:
  stage: lint
  image: golangci/golangci-lint:latest
  script:
    - echo "--- 运行 Go 代码规范检查 ---"
    - golangci-lint run ./... --timeout=5m
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
    - if: $CI_COMMIT_BRANCH == "main"
```

### 7.4 编辑器集成

#### VS Code / Trae IDE

在 `.vscode/settings.json` 中配置：

```json
{
    "go.lintTool": "golangci-lint",
    "go.lintFlags": ["--fast"],
    "go.lintOnSave": "package"
}
```

### 7.5 其他推荐工具

| 工具 | 用途 | 安装 |
|------|------|------|
| `godoc` | 本地预览生成的文档 | `go install golang.org/x/tools/cmd/godoc@latest` |
| `pkgsite` | 本地运行 pkg.go.dev 风格文档站 | `go install golang.org/x/pkgsite/cmd/pkgsite@latest` |
| `gomarkdoc` | 将 godoc 导出为 Markdown | `go install github.com/princjef/gomarkdoc/cmd/gomarkdoc@latest` |

本地预览文档：

```bash
# 使用 godoc
godoc -http=:6060
# 浏览器访问 http://localhost:6060/pkg/ling_flow/

# 使用 pkgsite
pkgsite -http=:8080
# 浏览器访问 http://localhost:8080/ling_flow/
```

---

## 附录：注释检查清单

在提交代码前，使用以下清单自查：

- [ ] 所有导出类型有以类型名开头的中文注释
- [ ] 所有导出函数/方法有以函数名开头的中文注释
- [ ] 所有导出接口有注释，接口方法也有注释
- [ ] 复杂函数的参数和返回值有说明
- [ ] 包的第一个文件有包级注释
- [ ] 枚举常量组有行尾注释
- [ ] 非直观的代码逻辑有解释性注释
- [ ] 长函数使用步骤分段标记
- [ ] godoc 注释以中文句号结尾
- [ ] 注释与声明之间无空行
- [ ] 未使用块注释 `/* */`
- [ ] `golangci-lint run ./...` 无注释相关警告
