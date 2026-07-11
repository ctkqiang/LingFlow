# WebSocket 协议规范 - 任务分解

## Phase 1: 连接层完善

### Task 1.1: 连接认证优化
- 文件: `internal/connections/wss.go`
- 完善 Token 验证逻辑
- 添加连接数限制（每 IP 和全局）
- 添加 Origin 检查日志
- 添加连接建立/断开的事件记录

### Task 1.2: 连接参数配置化
- 文件: `internal/utilities/config_loader.go`
- 添加心跳间隔配置项
- 添加心跳超时配置项
- 添加写入超时配置项
- 添加最大连接数配置项

---

## Phase 2: 消息模型标准化

### Task 2.1: 消息模型重构
- 文件: `internal/models/message.go`
- 整理所有消息类型常量
- 为每种消息类型定义对应的 Data 结构体
- 添加消息验证方法
- 添加 JSON 序列化辅助方法

### Task 2.2: 响应模型完善
- 文件: `internal/models/responses.go`
- 添加流式响应数据结构
- 添加 Token 用量统计结构
- 添加错误响应结构
- 添加技能列表响应结构

---

## Phase 3: 心跳机制实现

### Task 3.1: 心跳处理
- 文件: `internal/connections/wss.go`
- 实现 Ping/Pong 消息处理
- 实现心跳定时器
- 实现超时检测和断开逻辑
- 实现心跳重置机制

### Task 3.2: 写入超时控制
- 文件: `internal/connections/wss.go`
- 实现每条消息的写入超时
- 实现写入错误处理
- 添加超时日志记录

---

## Phase 4: 流式响应实现

### Task 4.1: 流式响应编排
- 文件: `internal/services/chat_handler.go`
- 实现流式响应分块逻辑
- 实现 chunk_index 序号管理
- 实现 is_final 标记
- 实现最终统计信息发送

### Task 4.2: 思考过程消息
- 文件: `internal/services/chat_handler.go`
- 实现技能选择阶段思考消息
- 实现 LLM 生成阶段思考消息
- 实现响应格式化阶段思考消息
- 确保思考消息的时序正确

---

## Phase 5: 错误处理统一

### Task 5.1: 错误码定义
- 文件: `internal/models/responses.go`
- 定义标准错误码常量
- 定义错误消息结构体
- 实现错误码与 HTTP 状态码映射
- 实现生产/开发环境错误信息切换

### Task 5.2: 错误处理重构
- 文件: `internal/services/chat_handler.go`
- 统一使用标准错误响应格式
- 添加消息格式验证错误处理
- 添加不支持消息类型的错误处理
- 添加技能不存在的错误处理

---

## Phase 6: 速率限制

### Task 6.1: 消息速率限制
- 文件: `internal/connections/wss.go` 或新建
- 实现每用户消息速率计数器
- 实现滑动窗口或令牌桶算法
- 实现超限拒绝和错误响应
- 配置速率限制参数

### Task 6.2: 技能创建速率限制
- 文件: `internal/services/skill_creator.go`
- 完善现有限制逻辑
- 统一使用速率限制组件
- 添加超限日志记录

---

## Phase 7: 技能列表推送

### Task 7.1: 初始推送
- 文件: `internal/connections/wss.go`
- 确保连接建立后立即推送技能列表
- 推送格式标准化
- 添加推送失败日志

### Task 7.2: 更新广播
- 文件: `internal/services/skill_creator.go`
- 创建技能成功后广播更新
- 通过 ConnectionManager 广播
- 确保所有活跃连接都收到更新

---

## Phase 8: 前端协议适配

### Task 8.1: 类型定义更新
- 文件: `demo/src/types/index.ts`
- 同步所有消息类型定义
- 添加流式响应类型
- 添加错误类型定义
- 添加心跳消息类型

### Task 8.2: 连接管理完善
- 文件: `demo/src/stores/chat.ts`
- 实现心跳发送机制
- 实现重连策略（指数退避）
- 实现连接状态监听
- 实现 Token 过期自动刷新

### Task 8.3: 消息处理完善
- 文件: `demo/src/stores/chat.ts`
- 完善流式消息拼接显示
- 完善思考阶段显示
- 完善错误消息处理
- 完善技能列表更新处理
