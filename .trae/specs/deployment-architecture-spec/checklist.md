# 部署架构规范 - 任务清单

## Kubernetes 部署

- [ ] 完善 Deployment 配置（副本数、资源限制）
- [ ] 完善 Service 配置
- [ ] 完善 Ingress 配置（WebSocket 支持、TLS）
- [ ] 完善 ConfigMap 配置
- [ ] 完善 Secret 配置模板
- [ ] 添加 HPA 配置
- [ ] 添加 PodDisruptionBudget 配置
- [ ] 添加 ServiceAccount + IRSA 配置
- [ ] 配置健康检查（存活探针、就绪探针）
- [ ] 配置滚动更新策略
- [ ] 配置 Pod 反亲和性
- [ ] 添加 NetworkPolicy（可选）

## Docker 部署

- [ ] 创建 Dockerfile（多阶段构建）
- [ ] 创建 docker-compose.yml（开发用）
- [ ] 优化镜像大小
- [ ] 配置非 root 用户运行
- [ ] 添加健康检查指令
- [ ] 添加 .dockerignore

## Lambda 部署

- [ ] 完善 Lambda handler
- [ ] 创建 Lambda 部署打包脚本
- [ ] 创建 API Gateway 配置示例
- [ ] 文档化 Lambda 限制和适用场景

## 配置管理

- [ ] 整理所有配置项
- [ ] 区分敏感/非敏感配置
- [ ] ConfigMap 配置完善
- [ ] Secret 配置模板完善
- [ ] 提供 Secrets Manager 集成方案
- [ ] 提供 IRSA 配置示例

## 监控可观测性

- [ ] 添加 /health 健康检查端点
- [ ] 添加 /ready 就绪检查端点
- [ ] 添加 /metrics 指标端点（Prometheus）
- [ ] 完善结构化日志
- [ ] 添加业务指标
- [ ] 提供 Grafana 仪表盘示例
- [ ] 提供告警规则示例

## CI/CD

- [ ] 完善 GitLab CI 配置
- [ ] 添加 GitHub Actions 工作流
- [ ] 配置代码检查阶段
- [ ] 配置测试阶段
- [ ] 配置构建阶段
- [ ] 配置部署阶段

## 文档

- [ ] K8s 部署步骤文档
- [ ] Docker 部署步骤文档
- [ ] Lambda 部署步骤文档
- [ ] 生产环境检查清单
- [ ] 监控配置文档
- [ ] 故障排查指南
- [ ] 性能调优指南

## 高可用

- [ ] 多 AZ 部署方案
- [ ] 自动扩缩容方案
- [ ] 滚动更新与回滚方案
- [ ] 故障自愈验证
- [ ] 灾难恢复方案

## 安全加固

- [ ] 镜像安全扫描
- [ ] 运行时安全配置
- [ ] 网络策略配置
- [ ] RBAC 权限配置
- [ ] 审计日志配置
