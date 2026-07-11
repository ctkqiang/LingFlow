# 部署架构规范

## Why

LingFlow 作为一个基于 Go + WebSocket 的 AI 对话系统，需要支持多种部署方式以适应不同的使用场景。目前代码中已实现了本地运行、Docker 和 Lambda 支持，但缺少完整的部署架构规范文档，导致：

1. Kubernetes 部署配置缺少详细的设计说明
2. 不同部署方式的优劣势和适用场景不明确
3. 水平扩展和高可用架构设计缺少规范
4. 监控和可观测性配置缺少标准
5. 生产环境部署 checklist 不完整

## What Changes

本 Spec 定义 LingFlow 部署架构的完整规范，包括：

- 部署方式对比与选型指南
- Kubernetes 部署架构设计
- 无服务器（Lambda）部署架构
- 水平扩展与高可用设计
- 配置与密钥管理
- 监控与可观测性
- 生产环境部署清单

### Impact

- Affected specs: 安全认证规范、WebSocket 协议规范
- Affected code:
  - `k8s/deployment.yaml` — K8s 部署配置
  - `k8s/service.yaml` — K8s 服务配置
  - `k8s/ingress.yaml` — K8s Ingress 配置
  - `k8s/configmap.yaml` — K8s 配置映射
  - `k8s/secret.yaml` — K8s 密钥配置
  - `internal/services/aws/lambda.go` — Lambda 处理器
  - `Dockerfile` — 容器镜像构建

## ADDED Requirements

### Requirement: 部署方式对比

系统 SHALL 支持多种部署方式，适用不同场景。

| 部署方式 | 适用场景 | 优点 | 缺点 |
|---------|---------|------|------|
| **本地运行** | 开发测试 | 简单直接，调试方便 | 不适合生产 |
| **Docker** | 单节点部署、快速验证 | 环境一致，部署简单 | 单实例，无高可用 |
| **Kubernetes** | 生产环境、大规模 | 高可用、弹性伸缩、自愈 | 复杂度高，运维成本高 |
| **AWS Lambda** | 低频调用、事件驱动 | 按需付费、免运维、自动扩展 | WebSocket 支持有限、冷启动延迟 |

#### 选型建议

- **开发/测试**：本地运行 或 Docker
- **小型生产（单节点）**：Docker + Nginx 反向代理
- **中大型生产（多节点）**：Kubernetes + Ingress
- **无状态 API 服务**：AWS Lambda（REST API 模式）

---

### Requirement: Kubernetes 部署架构

系统 SHALL 提供完整的 Kubernetes 部署配置。

#### 架构图

```
                    ┌─────────────────┐
                    │   DNS / CDN     │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  Ingress Nginx  │
                    │  (TLS 终止)      │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
        ┌─────▼─────┐  ┌────▼─────┐  ┌─────▼─────┐
        │  LingFlow  │  │ LingFlow │  │ LingFlow  │
        │  Pod 1     │  │ Pod 2    │  │ Pod 3     │
        └─────┬─────┘  └────┬─────┘  └─────┬─────┘
              │              │              │
              └──────────────┼──────────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
        ┌─────▼─────┐  ┌────▼─────┐  ┌─────▼─────┐
        │ Amazon    │  │ Amazon   │  │ Amazon    │
        │ S3        │  │ Bedrock  │  │ Secrets   │
        │ (技能存储) │  │ (LLM)    │  │ Manager   │
        └───────────┘  └──────────┘  └───────────┘
```

#### 核心组件

1. **Deployment** — LingFlow 应用部署
   - 副本数：默认 3 个
   - 滚动更新策略
   - 健康检查（存活探针 + 就绪探针）
   - 资源限制（CPU/Memory）

2. **Service** — 服务发现
   - 类型：ClusterIP
   - 端口：80 → 容器端口
   - 标签选择器匹配应用 Pod

3. **Ingress** — 入口路由
   - Nginx Ingress Controller
   - TLS 证书配置
   - WebSocket 支持（升级配置）
   - 负载均衡

4. **ConfigMap** — 非敏感配置
   - 环境变量
   - 应用配置

5. **Secret** — 敏感配置
   - AWS 凭证
   - API Key
   - JWT 密钥

#### Deployment 规格

| 资源 | 请求 (Requests) | 限制 (Limits) |
|------|----------------|--------------|
| CPU | 250m | 1000m |
| Memory | 256Mi | 512Mi |

#### 健康检查

- **存活探针** (Liveness Probe)：HTTP GET `/health`
  - 初始延迟：10s
  - 周期：10s
  - 超时：5s
  - 失败阈值：3

- **就绪探针** (Readiness Probe)：HTTP GET `/ready`
  - 初始延迟：5s
  - 周期：5s
  - 超时：3s
  - 失败阈值：3

#### 滚动更新策略

- `maxUnavailable`: 25%
- `maxSurge`: 25%
- 确保滚动更新期间服务不中断

#### Scenario: 滚动更新

- **GIVEN** 当前运行 3 个副本
- **WHEN** 部署新版本
- **THEN** 逐个替换 Pod，期间至少 75% 的副本可用，服务不中断

---

### Requirement: 水平扩展

系统 SHALL 支持水平扩展以应对流量变化。

#### HPA (Horizontal Pod Autoscaler)

- 基于 CPU 使用率自动扩缩容
- 扩容阈值：CPU 使用率 > 70%
- 缩容阈值：CPU 使用率 < 30%
- 最小副本数：2
- 最大副本数：10
- 扩容冷却：60s
- 缩容冷却：300s

#### 扩展指标（可选）

- CPU 使用率
- 内存使用率
- 自定义指标：WebSocket 连接数
- 自定义指标：每秒请求数 (RPS)

#### 扩展考虑因素

- **WebSocket 长连接**：扩容容易，缩容难（需优雅断开）
- **有状态 vs 无状态**：应用本身无状态，可水平扩展
- **S3/Bedrock 配额**：扩展时注意 AWS 服务配额限制
- **连接再平衡**：新 Pod 加入后，连接不会自动重新分布

#### Scenario: 流量高峰扩容

- **GIVEN** 流量高峰，CPU 使用率持续 > 70%
- **WHEN** HPA 检测到扩容条件
- **THEN** 自动增加 Pod 副本数，分担流量压力

---

### Requirement: AWS Lambda 部署架构

系统 SHALL 支持 Lambda 无服务器部署模式。

#### 架构图

```
    ┌──────────┐     ┌──────────┐     ┌───────────┐
    │  API     │────►│  Lambda  │────►│  DynamoDB │
    │ Gateway  │     │ LingFlow │     │  (可选)    │
    └──────────┘     └────┬─────┘     └───────────┘
                           │
                  ┌────────┴────────┐
                  │                 │
            ┌─────▼─────┐   ┌──────▼──────┐
            │ Amazon S3 │   │ Amazon      │
            │           │   │ Bedrock     │
            └───────────┘   └─────────────┘
```

#### Lambda 限制

- **无 WebSocket 支持**：API Gateway V2 的 WebSocket 支持需要额外处理
- **冷启动延迟**：首次调用或长时间空闲后有冷启动
- **执行时长限制**：最大 15 分钟（对话响应一般足够）
- **无状态**：每个请求独立，无法维护长连接状态

#### 适用场景

- 提供 REST API 接口的技能查询
- 异步技能创建任务
- 低频调用场景
- 事件驱动处理

#### Scenario: Lambda 处理技能查询

- **GIVEN** 用户通过 REST API 查询可用技能列表
- **WHEN** API Gateway 将请求转发给 Lambda
- **THEN** Lambda 从 S3 加载技能列表并返回，执行完成后容器释放

---

### Requirement: Docker 部署

系统 SHALL 提供 Docker 容器化部署支持。

#### Dockerfile 规范

- 多阶段构建
- 第一阶段：Go 编译
- 第二阶段：运行时镜像
- 使用 distroless 或 alpine 作为基础镜像
- 非 root 用户运行
- 暴露正确端口
- 健康检查指令

#### 镜像优化

- 镜像大小：< 50MB
- 层数最少化
- 依赖层缓存
- 只包含必要的二进制文件

#### Docker Compose（开发用）

- LingFlow 服务
- 可选：LocalStack（模拟 AWS 服务）
- 端口映射
- 环境变量配置
- 卷挂载（.env、技能文件）

---

### Requirement: 配置与密钥管理

系统 SHALL 遵循配置与密钥管理最佳实践。

#### 配置分层

| 层级 | 内容 | 存储方式 |
|------|------|---------|
| 应用配置 | 端口、日志级别、模式 | ConfigMap / .env |
| AWS 配置 | 区域、模型 ID、超时 | ConfigMap / 环境变量 |
| 敏感配置 | Access Key、API Key、JWT Secret | Secret / Secrets Manager |

#### Kubernetes 配置管理

- **ConfigMap**：存储非敏感配置
  - `MODE`
  - `AWS_REGION`
  - `AWS_BEDROCK_MODEL_ID`
  - `LOG_LEVEL`
  - 其他非敏感配置

- **Secret**：存储敏感配置
  - `AWS_ACCESS_KEY_ID`
  - `AWS_SECRET_ACCESS_KEY`
  - `API_KEY`
  - `JWT_SECRET`

- **推荐**：使用 AWS Secrets Manager + External Secrets Operator
  - 密钥集中管理
  - 自动轮换
  - 细粒度访问控制

#### IAM 角色（推荐）

- 在 EKS 中使用 IRSA (IAM Roles for Service Accounts)
- 无需在 Secret 中存储 AWS 凭证
- 使用临时凭证，更安全
- 最小权限原则

---

### Requirement: 监控与可观测性

系统 SHALL 提供监控和可观测性能力。

#### 日志

- **结构化日志**：JSON 格式，便于采集
- **日志级别**：DEBUG / INFO / WARN / ERROR
- **日志内容**：请求 ID、用户 ID、操作、耗时、错误信息
- **日志采集**：
  - K8s：ELK / EFK / Loki
  - AWS：CloudWatch Logs

#### 指标 (Metrics)

- **业务指标**：
  - 活跃 WebSocket 连接数
  - 每秒消息数
  - 技能调用次数
  - LLM 调用次数

- **系统指标**：
  - CPU 使用率
  - 内存使用率
  - Goroutine 数量
  - GC 次数和耗时

- **指标暴露**：
  - Prometheus 格式 (`/metrics`)
  - 或 AWS CloudWatch

#### 告警

- 服务不可用告警
- 错误率过高告警
- 资源使用率过高告警
- LLM 调用失败率告警
- S3 访问失败告警

#### 分布式追踪（可选）

- 使用 OpenTelemetry
- 追踪完整请求链路
- 识别性能瓶颈

---

### Requirement: 高可用设计

系统 SHALL 实现高可用部署架构。

#### 多副本部署

- 至少 2 个副本（生产环境建议 3+）
- 分布在不同可用区（AZ）
- 避免单点故障

#### 健康检查与自愈

- 存活探针：检测进程是否存活
- 就绪探针：检测是否可接收流量
- 失败自动重启 Pod
- 自动替换不健康实例

#### 滚动更新

- 零停机部署
- 逐步替换旧版本 Pod
- 新版本就绪后才摘除旧版本
- 可回滚到上一版本

#### 数据持久化

- 技能文件存储在 S3（高可用、持久化）
- 应用本身无状态，不存储数据
- 事件溯源使用内存存储（重启丢失，可选持久化）

---

### Requirement: 生产环境部署清单

系统 SHALL 提供生产环境部署前的检查清单。

#### 安全检查

- [ ] 启用生产模式 (`MODE=production`)
- [ ] 配置 HTTPS/TLS 证书
- [ ] 设置强 API Key 和 JWT Secret
- [ ] 配置 Origin 白名单
- [ ] 启用提示注入检测
- [ ] 配置速率限制
- [ ] 使用 IAM Role 替代 Access Key
- [ ] S3 存储桶禁用公共访问
- [ ] 错误信息不暴露内部细节
- [ ] 配置安全审计日志

#### 高可用检查

- [ ] 至少 3 个副本
- [ ] 分布在多个可用区
- [ ] 配置健康检查
- [ ] 配置 HPA 自动扩缩容
- [ ] 配置滚动更新策略
- [ ] 配置 Pod 反亲和性
- [ ] 设置资源请求和限制

#### 监控检查

- [ ] 配置日志采集
- [ ] 配置指标监控
- [ ] 配置告警规则
- [ ] 配置分布式追踪（可选）
- [ ] 设置日志保留策略

#### 性能检查

- [ ] 进行负载测试
- [ ] 验证最大连接数
- [ ] 验证响应延迟
- [ ] 验证 LLM 调用超时设置
- [ ] 验证 S3 访问性能

#### 备份与恢复

- [ ] S3 版本控制已启用
- [ ] 配置定期备份策略
- [ ] 验证恢复流程
- [ ] 文档化灾难恢复计划

---

### Requirement: CI/CD 流水线

系统 SHALL 提供 CI/CD 集成建议。

#### 流水线阶段

1. **代码检查**
   - 代码格式化检查
   - Lint 检查
   - 安全扫描

2. **测试**
   - 单元测试
   - 集成测试
   - 测试覆盖率

3. **构建**
   - 编译二进制
   - 构建 Docker 镜像
   - 推送镜像到仓库

4. **部署**
   - 开发环境自动部署
   - 预发环境手动审批
   - 生产环境手动审批
   - 滚动更新部署

#### 支持的 CI/CD 平台

- GitLab CI (`.gitlab-ci.yml`)
- GitHub Actions (`.github/workflows/`)
- Jenkins (可选)
- AWS CodePipeline (可选)
