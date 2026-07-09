# 安全政策

LingFlow 高度重视安全性。本文档描述了如何报告安全漏洞、项目的安全架构以及最佳实践建议。

---

## 目录

- [报告安全漏洞](#报告安全漏洞)
- [支持版本](#支持版本)
- [安全架构概览](#安全架构概览)
- [威胁模型](#威胁模型)
- [安全配置清单](#安全配置清单)
- [AWS 安全最佳实践](#aws-安全最佳实践)
- [提示注入防护](#提示注入防护)
- [事件响应流程](#事件响应流程)
- [安全审计](#安全审计)
- [致谢](#致谢)

---

## 报告安全漏洞

### 报告流程

**请不要通过公开的 GitHub Issue 报告安全漏洞。**

如发现安全漏洞，请通过以下方式私密报告：

1. **GitHub Security Advisory**（推荐）
   - 前往仓库 → Security → Advisories → New draft advisory
   - 填写漏洞描述、影响范围和复现步骤

2. **邮件报告**
   - 发送邮件至：`security@lingflow.dev`（如有）
   - 使用 PGP 加密（公钥见仓库 `docs/pgp-public-key.asc`，如有）

### 报告内容要求

请在报告中包含以下信息：

| 项目 | 说明 |
|------|------|
| **漏洞类型** | 如 SQL 注入、XSS、SSRF、认证绕过、提示注入等 |
| **影响范围** | 哪些组件受影响，攻击者可达到什么效果 |
| **复现步骤** | 详细到可直接操作的程度 |
| **环境信息** | LingFlow 版本、运行模式、配置摘要（隐藏敏感值） |
| **修复建议** | 如果你有建议的修复方案 |
| **报告者信息** | 姓名/昵称、联系方式（用于后续沟通和致谢） |

### 响应时间线

| 阶段 | 目标响应时间 |
|------|-------------|
| 确认收到报告 | 48 小时内 |
| 初步评估 | 5 个工作日内 |
| 修复方案通知 | 10 个工作日内 |
| 修复发布 | 根据严重程度，30-90 天内 |
| 公开披露 | 修复发布后 14 天 |

### 披露政策

- 我们遵循 **协同披露** 原则
- 修复发布前，漏洞详情不会公开
- 报告者可选择在修复发布后公开披露
- 我们会在修复公告中致谢报告者（除非要求匿名）

---

## 支持版本

| 版本 | 状态 | 安全更新支持 |
|------|------|-------------|
| 1.x.x | ✅ 当前版本 | 完整支持 |
| < 1.0 | ❌ 不再支持 | 请升级到最新版本 |

---

## 安全架构概览

### 多层防护体系

```
┌─────────────────────────────────────────────────────────────┐
│                     第 1 层：传输安全                         │
│  • TLS 1.2+ 强制启用（生产环境）                              │
│  • wss:// 协议加密所有 WebSocket 通信                         │
│  • HTTPS 加密所有 REST API 通信                               │
├─────────────────────────────────────────────────────────────┤
│                     第 2 层：认证与授权                        │
│  • HMAC-SHA256 Token 签名验证                                 │
│  • Token 过期机制（默认 24 小时）                              │
│  • API Key 验证（REST 接口）                                  │
│  • 开发/生产模式分离                                           │
├─────────────────────────────────────────────────────────────┤
│                     第 3 层：连接安全                         │
│  • Origin 白名单检查                                          │
│  • 单 IP 最大连接数限制（默认 10）                             │
│  • WebSocket 帧大小限制（64KB）                               │
│  • 心跳超时自动断开（默认 90 秒）                              │
├─────────────────────────────────────────────────────────────┤
│                     第 4 层：应用安全                         │
│  • 提示注入检测（输入层 + 输出层双重正则扫描）                  │
│  • 技能创建速率限制（每用户每分钟 5 次）                       │
│  • 技能名称严格校验（^[a-z0-9_]{1,64}$）                      │
│  • 技能内容安全校验（生成后扫描恶意代码模式）                    │
├─────────────────────────────────────────────────────────────┤
│                     第 5 层：数据安全                         │
│  • 生产环境隐藏内部错误信息                                    │
│  • AWS 凭证使用 IAM Role（不硬编码）                          │
│  • S3 存储桶权限最小化                                         │
│  • 敏感配置通过环境变量注入                                    │
├─────────────────────────────────────────────────────────────┤
│                     第 6 层：基础设施安全                      │
│  • AWS IAM 最小权限原则                                       │
│  • S3 存储桶加密（推荐 SSE-S3 或 SSE-KMS）                    │
│  • Bedrock 模型访问控制                                       │
│  • VPC 安全组限制入站流量                                     │
└─────────────────────────────────────────────────────────────┘
```

---

## 威胁模型

### 已识别威胁与缓解措施

| 威胁 | 风险等级 | 缓解措施 | 状态 |
|------|----------|----------|------|
| **提示注入攻击** | 高 | 输入层 8 类正则模式检测 + 输出层 7 类正则模式检测 | ✅ 已实现 |
| **WebSocket 劫持（CSWSH）** | 高 | Origin 白名单检查 | ✅ 已实现 |
| **认证绕过** | 高 | HMAC-SHA256 Token 签名 + 过期验证 | ✅ 已实现 |
| **凭证泄露** | 高 | 生产环境使用 IAM Role，不硬编码密钥 | ✅ 已实现 |
| **信息泄露** | 中 | 生产模式隐藏错误细节，返回通用错误消息 | ✅ 已实现 |
| **拒绝服务（DoS）** | 中 | 单 IP 连接数限制 + 帧大小限制 + 心跳超时 | ✅ 已实现 |
| **技能文件污染** | 中 | 两阶段提交 + AI 生成内容安全扫描 | ✅ 已实现 |
| **中间人攻击（MITM）** | 高 | TLS 强制启用（生产环境） | ✅ 已实现 |
| **S3 存储桶枚举** | 低 | IAM 策略限制 ListBucket 权限范围 | 📋 需配置 |
| **Token 重放攻击** | 中 | Token 过期机制 + Nonce 匹配 | ✅ 已实现 |

### 信任边界

```
┌─────────────────────────────────────────────────────────┐
│                    不可信区域                              │
│                                                         │
│  ┌─────────┐    ┌──────────┐    ┌──────────────────┐   │
│  │ 客户端   │    │ 用户输入  │    │ WebSocket 消息    │   │
│  └─────────┘    └──────────┘    └──────────────────┘   │
│                                                         │
└──────────────────────┬──────────────────────────────────┘
                       │ ← 信任边界 1：认证 + Origin + IP
┌──────────────────────▼──────────────────────────────────┐
│                   半可信区域                              │
│                                                         │
│  ┌──────────────┐  ┌──────────────────┐                │
│  │ Token 验证后  │  │ 技能创建命令      │                │
│  │ 的用户请求    │  │ (#create_skill)  │                │
│  └──────────────┘  └────────┬─────────┘                │
│                             │ ← 信任边界 2：提示注入检测
│                             │ + 速率限制 + 名称校验      │
└─────────────────────────────┼───────────────────────────┘
                              │
┌─────────────────────────────▼───────────────────────────┐
│                    可信区域                               │
│                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │ 内部服务调用  │  │ AWS SDK 调用  │  │ S3 读写操作   │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

---

## 安全配置清单

### 生产环境必填配置

```bash
# === 传输安全 ===
WSS_CERT_FILE=/path/to/cert.pem       # TLS 证书文件
WSS_KEY_FILE=/path/to/key.pem         # TLS 私钥文件

# === 认证安全 ===
WSS_AUTH_SECRET=your-hmac-secret-key   # HMAC Token 签名密钥（至少 32 字符）
AUTH_API_KEY=your-api-key              # REST API 认证密钥
AUTH_TOKEN_TTL=24h                     # Token 有效期

# === 连接安全 ===
WSS_ALLOWED_ORIGINS=https://yourdomain.com  # 允许的 Origin（逗号分隔）
WSS_MAX_CONNECTIONS_PER_IP=10               # 单 IP 最大连接数
WSS_ALLOW_ALL_ORIGINS=false                 # 禁止允许所有 Origin

# === 运行模式 ===
MODE=production                        # 生产模式

# === 心跳配置 ===
WSS_HEARTBEAT_INTERVAL=30s
WSS_HEARTBEAT_TIMEOUT=90s
WSS_HEARTBEAT_WRITE_TIMEOUT=10s
```

### 生产环境安全检查清单

- [ ] `MODE=production`（非 development）
- [ ] TLS 证书已配置且有效
- [ ] `WSS_AUTH_SECRET` 已设置为强随机字符串（至少 32 字符）
- [ ] `AUTH_API_KEY` 已设置为强随机字符串
- [ ] `WSS_ALLOWED_ORIGINS` 仅包含必要的域名
- [ ] `WSS_ALLOW_ALL_ORIGINS` 设为 `false`
- [ ] AWS 凭证使用 IAM Role（非硬编码 Access Key）
- [ ] S3 存储桶已启用加密（SSE-S3 或 SSE-KMS）
- [ ] S3 存储桶未设置为公开访问
- [ ] `.env` 文件已加入 `.gitignore`
- [ ] `.env` 文件不包含在 Docker 镜像中
- [ ] 日志中不包含敏感信息（Token、密钥等）
- [ ] Bedrock Guardrail 已评估和配置（如需要）
- [ ] 网络安全组仅开放必要端口

---

## AWS 安全最佳实践

### IAM 权限最小化

为 LingFlow 服务创建专用的 IAM 用户或角色，仅授予必要权限：

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:ListBucket",
        "s3:GetObject",
        "s3:PutObject",
        "s3:HeadObject",
        "s3:DeleteObject"
      ],
      "Resource": [
        "arn:aws:s3:::your-skill-bucket",
        "arn:aws:s3:::your-skill-bucket/*"
      ],
      "Condition": {
        "StringEquals": {
          "s3:prefix": ["skills/*"]
        }
      }
    },
    {
      "Effect": "Allow",
      "Action": [
        "bedrock:InvokeModel",
        "bedrock:Converse",
        "bedrock:ConverseStream"
      ],
      "Resource": "arn:aws:bedrock:*:*:model/*"
    }
  ]
}
```

### S3 存储桶安全

```bash
# 1. 阻止公开访问
aws s3api put-public-access-block \
  --bucket your-skill-bucket \
  --public-access-block-configuration \
    BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true

# 2. 启用默认加密
aws s3api put-bucket-encryption \
  --bucket your-skill-bucket \
  --server-side-encryption-configuration \
    '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}'

# 3. 禁用 ACL
aws s3api put-bucket-ownership-controls \
  --bucket your-skill-bucket \
  --ownership-controls Rules=[{ObjectOwnership=BucketOwnerEnforced}]

# 4. 启用版本控制（便于回滚恶意修改）
aws s3api put-bucket-versioning \
  --bucket your-skill-bucket \
  --versioning-configuration Status=Enabled
```

### Bedrock 安全

- 仅开通需要的模型
- 定期审查模型访问权限
- 启用 Bedrock Guardrail 进行内容过滤（设置 `ENABLE_BEDROCK_GUARDRAIL=true`）
- 配置 Guardrail 的内容过滤策略和拒绝词列表

### EC2 / Lambda 安全

```bash
# EC2: 使用 IAM Role 而非 Access Key
aws iam create-role --role-name LingFlowEC2Role --assume-role-policy-document file://trust-policy.json
aws iam attach-role-policy --role-name LingFlowEC2Role --policy-arn arn:aws:iam::ACCOUNT:policy/LingFlowPolicy

# Lambda: 配置执行角色
aws lambda update-function-configuration \
  --function-name LingFlow \
  --role arn:aws:iam::ACCOUNT:role/LingFlowLambdaRole
```

---

## 提示注入防护

### 防护机制

LingFlow 实施双层提示注入检测，保护 LLM 免受恶意输入影响：

**第一层：输入检测（用户消息 → LLM 之前）**

| 检测类别 | 模式示例 | 说明 |
|----------|----------|------|
| 指令覆盖 | `ignore`, `override`, `bypass`, `forget` | 尝试覆盖系统指令 |
| 系统提示 | `system.*prompt`, `hidden.*prompt` | 尝试获取系统提示 |
| 敏感信息 | `secret`, `password`, `api.?key`, `token` | 尝试获取敏感信息 |
| 代码执行 | `execute`, `eval`, `shell`, `command` | 尝试触发代码执行 |
| 恶意操作 | `inject`, `poison`, `corrupt`, `manipulate` | 恶意操作意图 |
| 文件操作 | `read.*file`, `write.*file`, `delete.*file` | 尝试操作文件系统 |
| 角色扮演 | `role.*play`, `simulate`, `pretend` | 尝试绕过角色限制 |
| 攻击意图 | `evil`, `malicious`, `attack`, `exploit` | 明确的攻击意图 |

**第二层：输出检测（LLM 输出 → 返回用户之前）**

| 检测类别 | 模式示例 | 说明 |
|----------|----------|------|
| 指令注入 | `system.*prompt`, `ignore.*previous` | 输出中包含注入指令 |
| 敏感泄露 | `secret`, `password`, `api.?key` | 输出中包含敏感信息 |
| 代码执行 | `execute`, `eval`, `shell` | 输出中包含可执行代码 |
| 文件操作 | `read.*file`, `write.*file` | 输出中包含文件操作 |
| 危险命令 | `rm -rf`, `sudo`, `chmod`, `curl.*pipe` | 输出中包含危险命令 |
| XSS 攻击 | `<script`, `javascript:`, `data:.*base64` | 输出中包含 XSS 载荷 |
| 编码绕过 | `\x`, `\u`, `\0` | 使用编码绕过检测 |

### 检测触发时的行为

1. **输入检测触发**：拒绝处理，返回 `prompt_injection_detected` 错误
2. **输出检测触发**：拒绝返回生成内容，返回 `output_injection_detected` 错误
3. **两次检测都触发**：记录安全事件日志，可选择通知管理员

### Bedrock Guardrail 集成

如需更强的防护，可启用 AWS Bedrock Guardrail：

```bash
# .env 配置
ENABLE_BEDROCK_GUARDRAIL=true
BEDROCK_GUARDRAIL_ID=your-guardrail-id
BEDROCK_GUARDRAIL_REGION=us-east-1
```

Bedrock Guardrail 提供：
- 内容分类过滤（仇恨、暴力、色情等）
- 拒绝词列表
- 敏感信息过滤（PII）
- 提示攻击防护
- 与 LingFlow 内置正则检测形成互补

---

## 事件响应流程

### 安全事件分级

| 级别 | 定义 | 响应时间 | 示例 |
|------|------|----------|------|
| **P0 - 紧急** | 正在被利用的漏洞，影响生产系统 | 1 小时内响应 | 认证绕过、RCE |
| **P1 - 高危** | 可被利用的漏洞，影响数据安全 | 4 小时内响应 | 提示注入绕过、信息泄露 |
| **P2 - 中危** | 需要特定条件才能利用 | 24 小时内响应 | 速率限制绕过、DoS |
| **P3 - 低危** | 安全加固建议 | 72 小时内响应 | 配置优化、日志改进 |

### 响应步骤

```
1. 发现 / 报告
   ↓
2. 分级评估（P0-P3）
   ↓
3. 临时缓解措施（如禁用功能、限制访问）
   ↓
4. 根因分析
   ↓
5. 开发修复方案
   ↓
6. 测试修复（单元测试 + 安全测试）
   ↓
7. 发布修复版本
   ↓
8. 事后分析报告（Post-mortem）
   ↓
9. 公开披露（如适用）
```

### 临时缓解措施

| 场景 | 临时措施 |
|------|----------|
| 认证绕过 | 临时关闭受影响端点，或添加 IP 白名单 |
| 提示注入绕过 | 临时禁用 `#create_skill` 功能 |
| S3 权限泄露 | 轮换 IAM 凭证，更新存储桶策略 |
| Token 泄露 | 缩短 Token TTL，强制所有用户重新认证 |

---

## 安全审计

### 日志审计

LingFlow 的结构化日志支持安全审计。关键审计事件：

| 审计事件 | 日志关键字段 | 审计目的 |
|----------|-------------|----------|
| 认证成功/失败 | user_id, source_ip, timestamp | 检测暴力破解 |
| Origin 拒绝 | origin, source_ip | 检测 CSWSH 攻击 |
| IP 连接数超限 | source_ip, connection_count | 检测 DoS 攻击 |
| 提示注入检测 | user_id, message, pattern_matched | 检测注入攻击 |
| 技能创建失败 | user_id, skill_name, failure_reason | 检测滥用行为 |
| 速率限制触发 | user_id, request_count | 检测自动化攻击 |
| S3 操作异常 | operation, error_code | 检测权限问题 |
| 心跳超时断开 | session_id, idle_duration | 检测僵尸连接 |

### 建议的日志收集方案

```bash
# 推荐使用 AWS CloudWatch 或 ELK 收集日志
# 关键告警规则：
# - 5 分钟内同一 IP 认证失败 > 10 次 → 潜在暴力破解
# - 5 分钟内同一用户技能创建失败 > 10 次 → 潜在滥用
# - 1 分钟内 Origin 拒绝 > 20 次 → 潜在 CSWSH 攻击
# - S3 操作 403 错误 > 5 次/分钟 → 权限配置问题
```

### 依赖安全扫描

```bash
# Go 依赖漏洞扫描
govulncheck ./...

# 或使用 Trivy
trivy fs --scanners vuln .
```

建议在 CI/CD 流水线中集成依赖扫描，阻断包含已知漏洞的 PR 合并。

---

## 致谢

感谢以下安全研究人员帮助提升 LingFlow 的安全性（按报告时间排序）：

<!-- 安全漏洞报告者将在修复发布后在此列出 -->

---

<div align="center">

**安全是所有人的责任**

如发现安全问题，请负责任地披露。

</div>
