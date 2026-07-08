# 调整 Event Sourcing 结构 Spec

## Why

当前 LingFlow 项目已经具备 Event Sourcing 的部分骨架（`DomainEvent`、`EventStore`、`Projection`），但核心业务流程（`ChatHandler` → `SkillExecutor` → `LLM`）完全绕过了事件溯源管道，导致：

1. 事件只在 WebSocket 连接层被记录，领域层（消息处理、技能执行、LLM 生成）不产生事件
2. 没有 Command/Event 分离——业务操作直接执行而非先验证再产生事件
3. 没有 Aggregate Root 来封装业务不变量和状态重建
4. 没有 EventBus 来在事件持久化后通知订阅者
5. Projection 只能手动重建，没有响应式维护
6. Lambda 模式完全不记录事件

## What Changes

- 在 `internal/events/` 下新增 `command.go`，定义 Command 基础结构和 Chat 领域命令
- 在 `internal/events/` 下新增 `aggregate.go`，定义 `ChatSessionAggregate` 聚合根
- 在 `internal/events/` 下新增 `bus.go`，定义 `EventBus` 接口和进程内实现
- 在 `internal/events/` 下新增 `handler.go`，定义 `CommandHandler` 将命令路由到聚合根
- 扩展 `event.go`，新增技能执行和 LLM 相关的 `EventType`
- 扩展 `chat_events.go`，新增对应的事件数据结构
- 调整 `chat_projection.go`，新增 `ChatSessionProjection.ApplyEvent` 方法支持增量投影
- 调整 `ChatHandler`，使其通过 Command → CommandHandler → Aggregate → Event 管道处理消息
- 调整 `WebsokcetConnectionManager`，使其通过 EventBus 触发广播而非直接调用
- 调整 Lambda handler，使其也记录事件

## Impact

- Affected specs: 事件溯源核心管道
- Affected code:
  - `internal/events/event.go` — 新增 EventType 常量
  - `internal/events/chat_events.go` — 新增事件数据结构
  - `internal/events/chat_projection.go` — 新增 ApplyEvent 方法
  - `internal/events/store.go` — 无改动
  - `internal/events/command.go` — **新增**
  - `internal/events/aggregate.go` — **新增**
  - `internal/events/bus.go` — **新增**
  - `internal/events/handler.go` — **新增**
  - `internal/connections/wss.go` — 重构为通过 EventBus 广播
  - `internal/services/chat_handler.go` — 重构为通过 Command 管道处理
  - `internal/services/server.go` — 组装 EventBus 和 CommandHandler
  - `internal/services/aws/lambda.go` — 通过 EventBus 记录事件

## ADDED Requirements

### Requirement: Command 模型

系统 SHALL 定义 Command 基础结构，包含 `CommandID`、`CommandType`、`AggregateID`、`IssuedAt`、`Data`、`Metadata` 字段。

系统 SHALL 定义以下 Chat 领域命令：
- `CommandTypeConnectSession` — 连接会话
- `CommandTypeDisconnectSession` — 断开会话
- `CommandTypeSendMessage` — 发送聊天消息

每个命令 SHALL 携带对应的命令数据结构（`ConnectSessionCommandData`、`DisconnectSessionCommandData`、`SendMessageCommandData`）。

#### Scenario: 构建命令成功

- **WHEN** 调用 `NewCommand` 传入合法参数
- **THEN** 返回完整的 Command 实例，`CommandID` 自动生成，`IssuedAt` 设为当前时间

### Requirement: Aggregate Root

系统 SHALL 定义 `ChatSessionAggregate` 聚合根，封装单个 chat session 的业务逻辑和状态。

聚合根 SHALL 从事件流重建状态：`RebuildChatSessionAggregate(eventHistory []DomainEvent) (*ChatSessionAggregate, error)`

聚合根 SHALL 提供以下命令方法，每个方法验证业务不变量后返回待持久化的事件列表：
- `HandleConnectSession(cmd Command) ([]DomainEvent, error)` — 若已连接则返回错误
- `HandleDisconnectSession(cmd Command) ([]DomainEvent, error)` — 若未连接则返回错误
- `HandleSendMessage(cmd Command) ([]DomainEvent, error)` — 若未连接则返回错误

聚合根 SHALL 提供 `ApplyEvent(event DomainEvent)` 方法，将事件应用到自身状态。

聚合根 SHALL 维护以下状态字段：
- `AggregateID string`
- `Connected bool`
- `ReceivedMessageCount int`
- `ProcessedMessageCount int`
- `FailedMessageCount int`
- `BroadcastMessageCount int`
- `CurrentVersion int64`

#### Scenario: 重建聚合根状态

- **GIVEN** 一个包含 `chat_session_connected` 和 `chat_message_received` 事件的事件流
- **WHEN** 调用 `RebuildChatSessionAggregate`
- **THEN** 聚合根状态为 `Connected=true`，`ReceivedMessageCount=1`

#### Scenario: 重复连接被拒绝

- **GIVEN** 聚合根已处于 Connected 状态
- **WHEN** 调用 `HandleConnectSession`
- **THEN** 返回错误，不产生事件

### Requirement: EventBus

系统 SHALL 定义 `EventBus` 接口，包含：
- `Subscribe(eventType EventType, handler EventHandler)` — 订阅特定事件类型
- `Publish(ctx context.Context, events ...DomainEvent) error` — 发布事件并通知所有订阅者

系统 SHALL 提供 `InMemoryEventBus` 进程内实现。

`EventHandler` 类型定义为 `func(ctx context.Context, event DomainEvent) error`。

#### Scenario: 事件发布后通知订阅者

- **GIVEN** 一个订阅了 `chat_message_processed` 事件的 handler
- **WHEN** 调用 `Publish` 发布该类型的事件
- **THEN** handler 被调用，接收到该事件

### Requirement: CommandHandler

系统 SHALL 定义 `ChatCommandHandler`，将命令路由到聚合根并协调事件持久化和发布。

`ChatCommandHandler` SHALL 依赖 `EventStore` 和 `EventBus`。

处理流程：
1. 从 `EventStore` 加载聚合根的事件流
2. 重建聚合根状态
3. 将命令交给聚合根处理，得到待持久化事件
4. 逐个追加事件到 `EventStore`
5. 通过 `EventBus` 发布所有已持久化事件

#### Scenario: 命令处理完整流程

- **GIVEN** 一个 `SendMessage` 命令和已连接的聚合根
- **WHEN** 调用 `ChatCommandHandler.Handle`
- **THEN** 事件被追加到 EventStore，EventBus 通知订阅者

### Requirement: 扩展事件类型

系统 SHALL 新增以下 `EventType` 常量：
- `EventTypeSkillExecutionStarted` — 技能执行开始
- `EventTypeSkillExecutionCompleted` — 技能执行完成
- `EventTypeSkillExecutionFailed` — 技能执行失败
- `EventTypeLLMGenerationStarted` — LLM 生成开始
- `EventTypeLLMGenerationCompleted` — LLM 生成完成
- `EventTypeLLMGenerationFailed` — LLM 生成失败

对应的事件数据结构：
- `SkillExecutionEventData` — 技能标识、查询文本、匹配分数
- `LLMGenerationEventData` — 技能标识、token 用量、延迟、停止原因

### Requirement: 增量投影

`ChatSessionProjection` SHALL 提供 `ApplyEvent(event DomainEvent) error` 方法，支持从当前投影状态增量应用单个事件。

系统 SHALL 提供 `NewChatSessionProjection(aggregateID string) *ChatSessionProjection` 构造函数。

#### Scenario: 增量应用事件

- **GIVEN** 一个初始的 `ChatSessionProjection`
- **WHEN** 连续调用 `ApplyEvent` 应用 `chat_session_connected` 和 `chat_message_received` 事件
- **THEN** 投影状态为 `Connected=true`，`ReceivedMessageCount=1`

### Requirement: ChatHandler 通过 Command 管道处理

`ChatHandler` SHALL 依赖 `ChatCommandHandler` 处理连接和消息命令，而非直接操作连接管理器。

`ChatHandler.HandleIncomingMessage` SHALL：
1. 解析消息后构建对应的 Command
2. 通过 `ChatCommandHandler.Handle` 处理命令
3. 返回处理结果

`ChatHandler` 不再直接调用 `SkillExecutor`，而是通过 EventBus 订阅 `chat_message_received` 事件来触发技能执行管道。

### Requirement: WebSocket 连接管理器通过 EventBus 广播

`WebsokcetConnectionManager` SHALL 订阅 `chat_message_processed` 事件，在收到事件时执行广播。

`WebsokcetConnectionManager.BroadcastMessage` 仍保留作为直接广播方法，但主流程通过 EventBus 事件驱动触发。

### Requirement: Lambda 模式记录事件

Lambda handler SHALL 在 `$connect`、`$disconnect`、`$default` 路由中记录对应事件到 EventStore。

## MODIFIED Requirements

### Requirement: 事件溯源核心管道

原先事件只在 `WebsokcetConnectionManager.recordEvent` 中被记录，缺少 Command/Aggregate/EventBus 层。

修改后：
- 所有业务操作通过 Command 发起
- CommandHandler 协调 Aggregate → EventStore → EventBus
- 订阅者通过 EventBus 响应事件
- Projection 通过 EventBus 增量维护
