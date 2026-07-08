# Checklist

## 事件溯源核心结构

- [ ] `Command` 结构体和 `CommandType` 已定义，`NewCommand` 能正确构建命令
- [ ] `ChatSessionAggregate` 能从事件流重建状态（`RebuildChatSessionAggregate`）
- [ ] `ChatSessionAggregate.ApplyEvent` 正确更新所有状态字段
- [ ] `HandleConnectSession` 验证不变量（已连接则拒绝），返回正确事件
- [ ] `HandleDisconnectSession` 验证不变量（未连接则拒绝），返回正确事件
- [ ] `HandleSendMessage` 验证不变量（未连接则拒绝），返回正确事件
- [ ] `EventBus` 接口和 `InMemoryEventBus` 实现线程安全的事件订阅和发布
- [ ] `ChatCommandHandler.Handle` 正确协调 Aggregate → EventStore → EventBus 流程

## 扩展事件类型

- [ ] 新增 6 个 `EventType` 常量已在 `event.go` 中定义
- [ ] `SkillExecutionEventData` 和 `LLMGenerationEventData` 已在 `chat_events.go` 中定义

## 增量投影

- [ ] `NewChatSessionProjection` 构造函数可用
- [ ] `ChatSessionProjection.ApplyEvent` 正确增量更新投影状态
- [ ] 原有 `RebuildChatSessionProjection` 仍可正常工作

## 业务层重构

- [ ] `ChatHandler` 通过 `ChatCommandHandler` 处理命令而非直接调用 SkillExecutor
- [ ] 技能执行管道通过 EventBus 订阅 `chat_message_received` 事件触发
- [ ] `WebsokcetConnectionManager` 订阅 `chat_message_processed` 事件执行广播
- [ ] `WebsokcetConnectionManager.BroadcastMessage` 直接方法仍可用
- [ ] `serveWebSocketHTTPServer` 正确组装 EventBus、CommandHandler、ChatHandler 及订阅

## Lambda 模式

- [ ] Lambda handler 在 `$connect` 路由记录 `chat_session_connected` 事件
- [ ] Lambda handler 在 `$disconnect` 路由记录 `chat_session_disconnected` 事件
- [ ] Lambda handler 在 `$default` 路由记录 `chat_message_received` 事件

## 代码质量

- [ ] 所有新增代码遵循项目现有编码风格（中文注释、错误消息中文）
- [ ] 不破坏现有功能——原有测试仍通过
- [ ] 新增代码均有单元测试覆盖
- [ ] 无硬编码密钥或敏感信息
