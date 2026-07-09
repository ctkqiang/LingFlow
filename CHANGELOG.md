# 变更日志

本项目所有重要变更均记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

---

## [Unreleased]

### 新增
- 完善开源项目文档（CONTRIBUTING.md、SECURITY.md、CODE_OF_CONDUCT.md）
- 添加 PlantUML 架构图源文件（architecture.puml、message_flow.puml）
- 添加 SVG Logo（docs/images/logo.svg）

---

## [1.0.0] - 2026-07-09

### 新增
- WebSocket 实时聊天通信框架
- S3 动态技能加载系统（Markdown 格式技能文件）
- AWS Bedrock LLM 集成（Converse API）
- Mock LLM 模式（开发测试用，无需 AWS 凭证）
- 流式响应机制（system_thinking + system_response 双阶段）
- 事件溯源架构（EventStore + Aggregate + EventBus）
- `#create_skill` AI 技能创建功能
  - 14 步创建流水线
  - 两阶段提交模式（空文件预留 + 内容写入）
  - 提示注入检测（输入层 8 类 + 输出层 7 类正则模式）
  - 速率限制（每用户每分钟 5 次）
  - 技能名称严格校验（^[a-z0-9_]{1,64}$）
- WebSocket 心跳机制
  - 双向 Ping/Pong（客户端发起 + 服务端发起）
  - 连接级超时检测（默认 90 秒）
  - 全局超时检测
  - 延迟统计与 Nonce 匹配
- 认证与授权系统
  - 开发模式（debug-token，免认证）
  - 生产模式（HMAC-SHA256 Token 签名）
  - REST API Token 获取接口
  - 预留生产认证逻辑扩展点
- 安全防护体系
  - TLS 强制启用（生产环境）
  - Origin 白名单检查
  - 单 IP 连接数限制（默认 10）
  - WebSocket 帧大小限制（64KB）
  - 生产环境错误信息屏蔽
- S3 技能加载器
  - 支持 ARN 格式存储桶名称解析
  - 支持 SKILLS_S3_BUCKET 和 AWS_SKILLS_S3_BUCKET 双变量
  - 支持 S3_REGION 独立区域配置
  - ListSkills / LoadSkill / UploadSkill / SkillExists / DeleteSkill / StorageURI
- 技能注册中心
  - 内存索引
  - 基于关键词的混合检索
  - 评分排序与阈值过滤
- 事件类型系统
  - 17 种领域事件类型
  - 会话生命周期事件
  - 心跳事件
  - 技能执行事件
  - LLM 生成事件
- 运行模式支持
  - Server 模式（本地 / EC2）
  - Lambda 模式（AWS API Gateway WebSocket 集成）
  - 优雅关闭（SIGINT/SIGTERM）
- 结构化日志系统
  - 5 级日志（DEBUG / VERBOSE / INFO / WARN / ERROR）
  - 中文进度消息
  - 性能指标记录
- 环境配置管理
  - 完整 .env 配置体系
  - S3 远程配置加载
  - AWS Secrets Manager 支持

### 安全
- 双层提示注入检测系统
- HMAC-SHA256 Token 签名认证
- 生产环境敏感信息屏蔽
- S3 IAM 最小权限策略
- 速率限制防滥用

---

## 版本号说明

```
MAJOR.MINOR.PATCH
```

| 版本段 | 变更类型 | 示例 |
|--------|----------|------|
| MAJOR | 不兼容的 API 变更 | 1.x.x → 2.0.0 |
| MINOR | 向下兼容的功能新增 | 1.0.x → 1.1.0 |
| PATCH | 向下兼容的 Bug 修复 | 1.0.0 → 1.0.1 |

## 变更类型说明

| 类型 | 说明 |
|------|------|
| `新增` | 新功能 |
| `变更` | 对已有功能的修改 |
| `弃用` | 即将移除的功能 |
| `移除` | 已移除的功能 |
| `修复` | Bug 修复 |
| `安全` | 安全相关修复 |
