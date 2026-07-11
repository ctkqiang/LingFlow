# 部署架构规范 - 任务分解

## Phase 1: Kubernetes 配置完善

### Task 1.1: 完善 Deployment
- 文件: `k8s/deployment.yaml`
- 添加存活探针和就绪探针
- 添加资源请求和限制
- 完善滚动更新策略
- 添加 Pod 反亲和性
- 添加环境变量引用 ConfigMap 和 Secret

### Task 1.2: 完善 Service 和 Ingress
- 文件: `k8s/service.yaml`, `k8s/ingress.yaml`
- 确认 Service 端口和选择器
- 完善 Ingress 配置
- 添加 WebSocket 升级支持
- 添加 TLS 配置示例

### Task 1.3: 添加 HPA 配置
- 新建文件: `k8s/hpa.yaml`
- 基于 CPU 使用率的自动扩缩容
- 配置最小/最大副本数
- 配置扩缩容阈值

---

## Phase 2: 配置管理完善

### Task 2.1: ConfigMap 完善
- 文件: `k8s/configmap.yaml`
- 整理所有非敏感配置
- 添加注释说明每个配置项
- 确保默认值合理

### Task 2.2: Secret 完善
- 文件: `k8s/secret.yaml`
- 整理所有敏感配置
- 使用 base64 编码示例
- 添加详细的填写说明
- 推荐使用 Secrets Manager

---

## Phase 3: Docker 支持

### Task 3.1: Dockerfile
- 新建文件: `Dockerfile`
- 多阶段构建
- 使用 Go 官方镜像编译
- 使用 alpine 或 distroless 作为运行时
- 非 root 用户运行
- 添加健康检查

### Task 3.2: .dockerignore
- 新建文件: `.dockerignore`
- 排除不必要的文件
- 优化构建上下文大小

### Task 3.3: docker-compose.yml（可选）
- 新建文件: `docker-compose.yml`
- LingFlow 服务
- 端口映射
- 环境变量
- 卷挂载

---

## Phase 4: 健康检查与监控端点

### Task 4.1: 健康检查端点
- 文件: `internal/services/server.go`
- 添加 `/health` 端点（存活探针）
- 添加 `/ready` 端点（就绪探针）
- 就绪探针检查：S3 连接、Bedrock 可用性

### Task 4.2: 指标端点（可选）
- 添加 `/metrics` 端点
- 暴露 Prometheus 格式指标
- 指标：连接数、消息数、LLM 调用数

---

## Phase 5: CI/CD

### Task 5.1: GitLab CI 完善
- 文件: `.gitlab-ci.yml`
- 添加构建阶段
- 添加 Docker 镜像构建
- 添加部署阶段

### Task 5.2: GitHub Actions
- 新建: `.github/workflows/ci.yml`
- 代码检查
- 单元测试
- 构建验证

---

## Phase 6: 部署文档

### Task 6.1: K8s 部署指南
- 文档: README.md 或独立文档
- 前提条件
- 部署步骤
- 配置说明
- 验证方法
- 常见问题

### Task 6.2: 生产环境检查清单
- 安全检查
- 高可用检查
- 监控检查
- 性能检查
- 备份检查

---

## Phase 7: Lambda 完善（可选）

### Task 7.1: Lambda Handler
- 文件: `internal/services/aws/lambda.go`
- 确保 REST API 模式正常工作
- 添加 API Gateway 事件处理
- 添加响应格式化

### Task 7.2: 部署脚本
- 新建 Lambda 打包脚本
- 文档化部署步骤
