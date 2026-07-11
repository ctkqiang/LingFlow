# 安全认证规范 - 任务分解

## Phase 1: 认证机制完善

### Task 1.1: 认证服务重构
- 文件: `internal/services/auth_handler.go`
- 整理开发模式认证逻辑
- 完善生产模式认证框架
- 添加清晰的扩展点注释
- 实现 Token 签发逻辑
- 实现 Token 验证逻辑

### Task 1.2: Token 模型定义
- 文件: `internal/models/`
- 定义 Token 数据结构
- 定义 Token 声明（claims）
- 实现 Token 序列化/反序列化

---

## Phase 2: 网络层防护

### Task 2.1: Origin 检查
- 文件: `internal/connections/wss.go`
- 完善 Origin 验证逻辑
- 添加配置项支持多域名
- 开发模式允许所有 Origin
- 生产模式严格校验
- 添加 Origin 检查失败日志

### Task 2.2: 连接数限制
- 文件: `internal/connections/wss.go`
- 实现每 IP 连接数计数
- 实现全局连接数计数
- 超限时拒绝新连接
- 返回合适的错误信息
- 断开时释放计数

---

## Phase 3: 速率限制实现

### Task 3.1: 速率限制器
- 文件: `internal/utilities/` 新建
- 实现通用速率限制器
- 支持滑动窗口算法
- 支持每用户/每 IP 维度
- 支持配置限制参数

### Task 3.2: 消息速率限制
- 文件: `internal/connections/wss.go`
- 集成速率限制器
- 每条消息前检查
- 超限时拒绝并返回 429
- 记录速率限制事件

### Task 3.3: 技能创建速率限制
- 文件: `internal/services/skill_creator.go`
- 集成速率限制器
- 创建前检查速率
- 超限返回错误
- 记录安全事件

---

## Phase 4: 输入验证强化

### Task 4.1: 消息验证
- 文件: `internal/models/message.go`
- 添加消息验证方法
- 验证 JSON 格式
- 验证必填字段
- 验证字段类型
- 验证消息大小

### Task 4.2: 技能名称验证
- 文件: `internal/services/skill_creator.go`
- 完善名称格式验证
- 长度限制（1-64字符）
- 字符集限制（小写字母、数字、下划线）
- 空值检查

---

## Phase 5: 提示注入防护

### Task 5.1: 注入检测引擎
- 文件: `internal/services/` 新建或扩展
- 定义注入模式正则
- 实现指令覆盖检测
- 实现角色扮演检测
- 实现内容泄露检测
- 实现置信度评分

### Task 5.2: 输入层集成
- 文件: `internal/services/chat_handler.go`
- 消息处理前调用检测
- 超过阈值则拒绝
- 返回安全警告
- 记录安全事件

### Task 5.3: 输出层审查
- 文件: `internal/services/llm.go`
- LLM 响应后审查
- 检查敏感信息泄露
- 检查系统指令泄露
- 发现问题则替换为安全响应

### Task 5.4: Bedrock Guardrail 集成
- 文件: `internal/services/llm.go`
- 添加 Guardrail 配置项
- 实现输入过滤调用
- 实现输出过滤调用
- 处理 Guardrail 拦截响应

---

## Phase 6: 错误信息保护

### Task 6.1: 错误响应统一
- 文件: `internal/models/responses.go`
- 定义标准错误响应结构
- 定义错误码枚举
- 实现生产/开发模式切换

### Task 6.2: 错误处理重构
- 文件: `internal/connections/wss.go`, `internal/services/`
- 统一使用标准错误响应
- 生产环境隐藏详细信息
- 开发环境返回完整错误
- 服务端记录完整错误日志

---

## Phase 7: 审计日志

### Task 7.1: 安全日志定义
- 文件: `internal/utilities/logger.go`
- 定义安全日志级别
- 定义安全事件类型
- 实现结构化日志输出
- 添加日志辅助方法

### Task 7.2: 日志埋点
- 认证日志: `internal/services/auth_handler.go`
- 连接日志: `internal/connections/wss.go`
- 操作日志: `internal/services/skill_creator.go`
- 安全事件: 各防护模块

---

## Phase 8: 配置管理

### Task 8.1: 安全配置项
- 文件: `internal/utilities/config_loader.go`
- 添加所有安全配置项
- 设置合理的默认值
- 实现配置验证
- 提供配置文档

---

## Phase 9: 测试

### Task 9.1: 单元测试
- 认证逻辑测试
- 注入检测测试
- 速率限制测试
- 输入验证测试
- 错误隐藏测试

### Task 9.2: 集成测试
- WebSocket 连接安全测试
- 技能创建安全测试
- 注入攻击拦截测试
