# Tasks

- [ ] Task 1: 新增 Command 模型 (`internal/events/command.go`)
  - [ ] 1.1: 定义 `CommandType` 类型和三个命令常量（`CommandTypeConnectSession`、`CommandTypeDisconnectSession`、`CommandTypeSendMessage`）
  - [ ] 1.2: 定义 `Command` 结构体（`CommandID`、`CommandType`、`AggregateID`、`IssuedAt`、`Data`、`Metadata`）
  - [ ] 1.3: 定义 `ConnectSessionCommandData`、`DisconnectSessionCommandData`、`SendMessageCommandData` 数据结构
  - [ ] 1.4: 实现 `NewCommand` 构造函数（自动生成 `CommandID`，设置 `IssuedAt`）

- [ ] Task 2: 新增 Aggregate Root (`internal/events/aggregate.go`)
  - [ ] 2.1: 定义 `ChatSessionAggregate` 结构体及状态字段
  - [ ] 2.2: 实现 `ApplyEvent` 方法，将事件应用到聚合根状态
  - [ ] 2.3: 实现 `RebuildChatSessionAggregate` 从事件流重建聚合根
  - [ ] 2.4: 实现 `HandleConnectSession` 命令方法（验证不变量，返回事件）
  - [ ] 2.5: 实现 `HandleDisconnectSession` 命令方法
  - [ ] 2.6: 实现 `HandleSendMessage` 命令方法

- [ ] Task 3: 新增 EventBus (`internal/events/bus.go`)
  - [ ] 3.1: 定义 `EventHandler` 函数类型 `func(ctx context.Context, event DomainEvent) error`
  - [ ] 3.2: 定义 `EventBus` 接口（`Subscribe`、`Publish`）
  - [ ] 3.3: 实现 `InMemoryEventBus`（线程安全，按事件类型分发）

- [ ] Task 4: 新增 CommandHandler (`internal/events/handler.go`)
  - [ ] 4.1: 定义 `ChatCommandHandler` 结构体（依赖 `EventStore` 和 `EventBus`）
  - [ ] 4.2: 实现 `Handle` 方法：加载事件流 → 重建聚合根 → 执行命令 → 持久化事件 → 发布事件

- [ ] Task 5: 扩展事件类型和事件数据
  - [ ] 5.1: 在 `event.go` 新增 6 个 `EventType` 常量（`EventTypeSkillExecutionStarted` 等）
  - [ ] 5.2: 在 `chat_events.go` 新增 `SkillExecutionEventData` 和 `LLMGenerationEventData` 数据结构

- [ ] Task 6: 增量投影支持 (`chat_projection.go`)
  - [ ] 6.1: 新增 `NewChatSessionProjection` 构造函数
  - [ ] 6.2: 新增 `ApplyEvent` 方法，支持单事件增量投影
  - [ ] 6.3: 保留原有 `RebuildChatSessionProjection` 不变

- [ ] Task 7: 重构 ChatHandler 使用 Command 管道
  - [ ] 7.1: `ChatHandler` 新增 `ChatCommandHandler` 依赖
  - [ ] 7.2: `HandleIncomingMessage` 构建命令并交给 `ChatCommandHandler`
  - [ ] 7.3: 保留消息解析和响应格式化逻辑
  - [ ] 7.4: 在 EventBus 上订阅 `chat_message_received` 触发技能执行管道

- [ ] Task 8: 重构 WebSocket 连接管理器使用 EventBus
  - [ ] 8.1: `WebsokcetConnectionManager` 新增 `EventBus` 依赖
  - [ ] 8.2: 订阅 `chat_message_processed` 事件触发广播
  - [ ] 8.3: 保留 `BroadcastMessage` 作为直接广播方法

- [ ] Task 9: 调整 Server 组装逻辑
  - [ ] 9.1: `serveWebSocketHTTPServer` 中创建 `InMemoryEventBus`
  - [ ] 9.2: 组装 `ChatCommandHandler`（EventStore + EventBus）
  - [ ] 9.3: 组装 `ChatHandler`（注入 CommandHandler + SkillExecutor）
  - [ ] 9.4: 注册 EventBus 订阅（技能执行、广播、投影维护）

- [ ] Task 10: Lambda 模式记录事件
  - [ ] 10.1: Lambda handler 注入 `EventStore` 依赖
  - [ ] 10.2: `$connect` 路由记录 `chat_session_connected` 事件
  - [ ] 10.3: `$disconnect` 路由记录 `chat_session_disconnected` 事件
  - [ ] 10.4: `$default` 路由记录 `chat_message_received` 事件

- [ ] Task 11: 编写单元测试
  - [ ] 11.1: `command_test.go` — 测试命令构建
  - [ ] 11.2: `aggregate_test.go` — 测试聚合根重建、命令处理、不变量验证
  - [ ] 11.3: `bus_test.go` — 测试事件发布和订阅
  - [ ] 11.4: `handler_test.go` — 测试 CommandHandler 完整流程
  - [ ] 11.5: `projection_test.go` — 测试增量投影

# Task Dependencies

- Task 1 (Command) → Task 2 (Aggregate), Task 4 (CommandHandler)
- Task 2 (Aggregate) → Task 4 (CommandHandler)
- Task 3 (EventBus) → Task 4 (CommandHandler), Task 7 (ChatHandler), Task 8 (Connection Manager), Task 9 (Server)
- Task 4 (CommandHandler) → Task 7 (ChatHandler), Task 9 (Server)
- Task 5 (扩展事件类型) → Task 7 (ChatHandler), Task 8 (Connection Manager)
- Task 6 (增量投影) → Task 9 (Server)
- Task 7, 8 → Task 9 (Server 组装)
- Task 9 → Task 10 (Lambda)
- Task 1-6 → Task 11 (测试)

可并行: Task 1, Task 3, Task 5, Task 6 可并行开发
