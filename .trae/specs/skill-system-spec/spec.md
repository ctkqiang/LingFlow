# 技能系统规范

## Why

LingFlow 项目的核心能力之一是动态技能系统，技能以 Markdown 文件形式存储在 S3 中，运行时动态加载并匹配用户请求。目前代码中已实现了技能加载、匹配、执行和创建的基本功能，但缺乏统一的规范文档，导致：

1. 技能文件格式不统一，缺少明确的元数据字段定义
2. 技能匹配算法缺少清晰的评分规则说明
3. 技能创建流程的安全边界定义不明确
4. 技能执行的上下文注入机制缺少标准化约定
5. 新贡献者难以快速理解如何编写技能

## What Changes

本 Spec 定义 LingFlow 技能系统的完整规范，包括：

- 技能文件格式标准（Markdown + Front Matter 元数据）
- 技能检索与匹配算法规范
- 技能执行与上下文注入机制
- 技能创建流水线与安全防护
- 技能版本管理与兼容性策略

### Impact

- Affected specs: 事件溯源架构、安全认证规范
- Affected code:
  - `internal/services/skill_registry.go` — 技能注册表与匹配逻辑
  - `internal/services/skill_executor.go` — 技能执行器
  - `internal/services/skill_creator.go` — 技能创建流水线
  - `internal/services/s3_skill_loader.go` — S3 技能加载器
  - `internal/models/skills.go` — 技能数据模型

## ADDED Requirements

### Requirement: 技能文件格式标准

系统 SHALL 定义标准的技能文件格式，基于 Markdown + YAML Front Matter。

技能文件结构：

```markdown
---
id: vulnerability_scanner
name: 漏洞扫描器
display_name: 漏洞扫描分析
version: 1.0.0
description: 分析系统安全漏洞，提供漏洞评估和修复建议
author: LingFlow Team
category: security
tags:
  - 漏洞扫描
  - 安全评估
  - 渗透测试
keywords:
  - 漏洞
  - 扫描
  - 安全
  - 渗透测试
  - vulnerability
  - scan
  - security
priority: 1
created_at: 2024-01-01T00:00:00Z
updated_at: 2024-01-01T00:00:00Z
---

## 技能描述

你是一名专业的网络安全专家，擅长漏洞扫描和安全评估...

## 能力范围

- 系统漏洞分析
- Web 应用安全检测
- 网络安全评估
- 修复建议提供

## 输出格式

回答时请按照以下结构组织：
1. 漏洞概述
2. 风险等级
3. 技术细节
4. 修复建议
```

#### Front Matter 字段说明

系统 SHALL 支持以下 Front Matter 字段：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 技能唯一标识，小写字母数字下划线，1-64字符 |
| `name` | string | 是 | 技能内部名称 |
| `display_name` | string | 是 | 技能展示名称（中文） |
| `version` | string | 是 | 语义化版本号（MAJOR.MINOR.PATCH） |
| `description` | string | 是 | 技能简短描述（用于匹配） |
| `author` | string | 否 | 技能作者 |
| `category` | string | 否 | 技能分类 |
| `tags` | string[] | 否 | 技能标签列表 |
| `keywords` | string[] | 是 | 关键词列表（用于匹配） |
| `priority` | number | 否 | 优先级，默认 1，数值越高优先级越高 |
| `created_at` | string | 否 | 创建时间（ISO 8601） |
| `updated_at` | string | 否 | 更新时间（ISO 8601） |

#### Scenario: 解析合法技能文件

- **GIVEN** 一个包含正确 Front Matter 的 Markdown 文件
- **WHEN** 调用 `LoadSkill` 加载该文件
- **THEN** 返回完整的 Skill 对象，所有字段正确解析

#### Scenario: 缺少必填字段

- **GIVEN** 一个缺少 `id` 或 `keywords` 字段的技能文件
- **WHEN** 尝试加载该技能
- **THEN** 返回错误，说明缺少的必填字段

---

### Requirement: 技能检索与匹配算法

系统 SHALL 实现基于关键词和描述的技能匹配算法。

#### 匹配流程

1. **文本归一化**
   - 用户消息转换为小写
   - 中文分词（可选）
   - 去除停用词和标点符号

2. **多维度评分**
   - **关键词匹配分**（权重 0.5）：命中关键词数量 / 总关键词数量
   - **描述匹配分**（权重 0.3）：描述文本与查询的相似度
   - **显示名称匹配分**（权重 0.2）：显示名称的精确/模糊匹配

3. **综合评分公式**

```
final_score = keyword_score * 0.5 + description_score * 0.3 + display_name_score * 0.2
```

4. **过滤与排序**
   - 过滤掉综合评分低于阈值（默认 0.3）的结果
   - 按综合评分降序排序
   - 返回 Top-N 结果（默认 5 个）

5. **优先级调整**
   - 相同评分时，`priority` 字段值高的排前面
   - `priority` 影响：`final_score = final_score * (1 + priority * 0.1)`

#### Scenario: 精确关键词匹配

- **GIVEN** 用户消息包含 "漏洞扫描"，技能 `/vulnerability_scanner` 的 keywords 包含该词
- **WHEN** 调用 `SkillRegistry.Retrieve("检测系统安全漏洞")`
- **THEN** 返回的技能列表中 `/vulnerability_scanner` 排在第一位，评分大于 0.8

#### Scenario: 低于阈值的结果被过滤

- **GIVEN** 用户消息与某技能的匹配度为 0.2
- **WHEN** 执行技能检索
- **THEN** 该技能不出现在结果列表中

#### Scenario: 优先级影响排序

- **GIVEN** 两个技能评分相同，技能 A 的 priority=2，技能 B 的 priority=1
- **WHEN** 返回匹配结果
- **THEN** 技能 A 排在技能 B 前面

---

### Requirement: 技能执行与上下文注入

系统 SHALL 在 LLM 调用时注入技能的 System Prompt 上下文。

#### 注入方式

- 将技能的完整 Markdown 内容作为 System Message 的一部分
- 保持原始的系统指令，追加技能特定指令
- 技能内容置于系统指令之后、用户消息之前

#### 注入结构

```
[System Instructions]
    |
    v
[Skill Context]  <-- 技能 Markdown 内容
    |
    v
[User Message]
```

#### Scenario: 技能上下文正确注入

- **GIVEN** 用户选择了 `/vulnerability_scanner` 技能
- **WHEN** 执行 LLM 调用
- **THEN** 请求中包含该技能的完整描述和能力范围作为系统上下文

---

### Requirement: 技能创建流水线

系统 SHALL 提供通过 `#create_skill` 命令创建新技能的能力。

#### 14 步流水线

1. **命令解析** — 从用户消息中提取技能名称和描述
2. **名称验证** — 仅允许小写字母、数字、下划线，长度 1-64 字符
3. **速率限制检查** — 每用户每分钟最多 5 次创建尝试
4. **提示注入检测（输入层）** — 正则匹配常见注入模式
5. **技能存在性检查** — S3 HeadObject 判断是否已存在
6. **创建空占位文件** — 上传空文件到 S3 预留名称（两阶段提交）
7. **AI 生成技能内容** — 调用 Bedrock 生成 Markdown 内容
8. **提示注入检测（输出层）** — 审查生成的内容
9. **元数据注入** — 添加 Front Matter 元数据
10. **内容验证** — 验证生成内容的格式和完整性
11. **覆盖上传到 S3** — 用完整内容替换占位文件
12. **更新本地技能注册表** — 重新加载技能列表
13. **推送更新后的技能列表** — 通知所有连接的客户端
14. **返回创建成功响应** — 告知用户创建结果

#### 安全开关

- 由 `IS_ALLOW_USER_CREATE_SKILL` 环境变量控制
- 默认值为 `false`（关闭）
- 生产环境默认关闭，需要显式启用

#### Scenario: 成功创建技能

- **GIVEN** 用户发送 `#create_skill network_scanner 网络扫描工具`，且功能已启用
- **WHEN** 执行完整的创建流水线
- **THEN** S3 中创建了 `skills/network_scanner.md`，内容包含 AI 生成的技能描述，客户端收到技能列表更新

#### Scenario: 功能未启用时拒绝创建

- **GIVEN** `IS_ALLOW_USER_CREATE_SKILL=false`
- **WHEN** 用户尝试使用 `#create_skill` 命令
- **THEN** 返回 403 错误（生产环境隐藏具体原因）

---

### Requirement: 技能版本管理

系统 SHALL 支持技能的版本管理和兼容性。

#### 版本号规范

- 使用语义化版本（SemVer）：`MAJOR.MINOR.PATCH`
- MAJOR：不兼容的 API 变更
- MINOR：向下兼容的功能性新增
- PATCH：向下兼容的问题修正

#### 兼容性策略

- 加载技能时验证版本格式
- 同一技能 ID 只保留最新版本
- 重大变更时创建新的技能 ID

#### Scenario: 加载不同版本技能

- **GIVEN** S3 中有两个同名不同版本的技能文件
- **WHEN** 加载技能列表
- **THEN** 只保留版本号最新的那个

---

### Requirement: 技能分类与标签

系统 SHALL 支持技能的分类和标签体系。

#### 预定义分类

- `security` — 网络安全相关
- `analysis` — 数据分析相关
- `development` — 开发工具相关
- `general` — 通用助手

#### 标签系统

- 每个技能可设置多个标签
- 标签用于筛选和展示
- 标签不参与匹配评分

---

### Requirement: 技能加载与缓存

系统 SHALL 实现高效的技能加载和缓存机制。

#### 加载时机

- 服务启动时全量加载
- 创建新技能后增量刷新
- 支持定时全量刷新（可选）

#### 缓存策略

- 内存缓存所有已加载的技能
- 技能内容懒加载（列表先加载元数据，详情按需加载）
- S3 访问失败时使用缓存兜底

#### Scenario: 服务启动时加载技能

- **GIVEN** S3 中有 10 个技能文件
- **WHEN** 服务启动并初始化 SkillRegistry
- **THEN** 10 个技能全部加载到内存，技能列表可用于匹配
