package services

import (
	"context"
	"encoding/json"
	"fmt"
	"ling_flow/internal/models"
	"ling_flow/internal/utilities"
	"regexp"
	"strings"
	"time"
)

const (
	createSkillCommandPrefix     = "#create_skill"
	maxGeneratedSkillSize        = 50 * 1024
	createSkillDescriptionMinLen = 5
	skillCreationPreviewSize     = 500
	maxCreateSkillRatePerMinute  = 5
	maxDescriptionLength         = 1000
)

var skillNameRegex = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

var (
	injectionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)ignore|override|bypass|disregard|forget|cancel`),
		regexp.MustCompile(`(?i)system.*prompt|instructions.*override|hidden.*prompt`),
		regexp.MustCompile(`(?i)secret|password|api.?key|token|credentials`),
		regexp.MustCompile(`(?i)execute|run.*code|eval|shell|command`),
		regexp.MustCompile(`(?i)inject|poison|corrupt|manipulate`),
		regexp.MustCompile(`(?i)read.*file|write.*file|delete.*file|access.*data`),
		regexp.MustCompile(`(?i)role.*play|simulate|pretend|as.*if`),
		regexp.MustCompile(`(?i)evil|malicious|attack|exploit`),
	}

	outputInjectionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)system.*prompt|instructions.*override|ignore.*previous`),
		regexp.MustCompile(`(?i)secret|password|api.?key|token|credentials`),
		regexp.MustCompile(`(?i)execute|run.*code|eval|shell|command`),
		regexp.MustCompile(`(?i)read.*file|write.*file|delete.*file`),
		regexp.MustCompile(`(?i)rm\s+-rf|sudo|chmod|curl.*pipe|wget.*pipe`),
		regexp.MustCompile(`(?i)<script|javascript:|data:.*base64`),
		regexp.MustCompile(`(?i)\\x|\\u|\\0|\\r|\\n\s*\\n`),
	}
)

type PromptInjectionDetection struct {
	IsInjection     bool
	MatchedPatterns []string
	Confidence      float64
	Reason          string
}

type CreateSkillRateLimiter struct {
	requestCounts map[string]int
	lastReset     time.Time
	mutex         chan struct{}
}

func NewCreateSkillRateLimiter() *CreateSkillRateLimiter {
	return &CreateSkillRateLimiter{
		requestCounts: make(map[string]int),
		lastReset:     time.Now(),
		mutex:         make(chan struct{}, 1),
	}
}

func (limiter *CreateSkillRateLimiter) Allow(userID string) bool {
	const component = "SkillCreator"
	const op = "RateLimiter.Allow"
	traceID := utilities.NewTraceID()

	limiter.mutex <- struct{}{}
	defer func() { <-limiter.mutex }()

	now := time.Now()
	windowReset := false
	if now.Sub(limiter.lastReset) > time.Minute {
		limiter.requestCounts = make(map[string]int)
		limiter.lastReset = now
		windowReset = true
	}

	count := limiter.requestCounts[userID]
	allowed := count < maxCreateSkillRatePerMinute

	utilities.LogVerbose(component, op, "速率限制检查",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("user_id=%s", utilities.Mask(userID)),
		fmt.Sprintf("current_count=%d", count),
		fmt.Sprintf("limit=%d", maxCreateSkillRatePerMinute),
		fmt.Sprintf("window_reset=%t", windowReset),
		fmt.Sprintf("allowed=%t", allowed),
		fmt.Sprintf("window_age_ms=%d", now.Sub(limiter.lastReset).Milliseconds()),
	)

	if !allowed {
		utilities.LogWarn(component, op,
			"速率限制已触发，拒绝请求",
			0,
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("user_id=%s", utilities.Mask(userID)),
			fmt.Sprintf("count=%d", count),
			fmt.Sprintf("limit=%d", maxCreateSkillRatePerMinute),
		)
		return false
	}

	limiter.requestCounts[userID] = count + 1
	return true
}

func parseCreateSkillCommand(message string) (skillName string, description string, isCommand bool) {
	const component = "SkillCreator"
	const op = "parseCreateSkillCommand"
	traceID := utilities.NewTraceID()

	trimmed := strings.TrimSpace(message)

	utilities.LogVerbose(component, op, "检测是否为创建技能命令",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("message_len=%d", len(trimmed)),
		fmt.Sprintf("has_prefix=%t", strings.HasPrefix(trimmed, createSkillCommandPrefix)),
	)

	if !strings.HasPrefix(trimmed, createSkillCommandPrefix) {
		utilities.LogVerbose(component, op, "非创建技能命令，跳过",
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return "", "", false
	}

	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, createSkillCommandPrefix))
	if rest == "" {
		utilities.LogVerbose(component, op, "检测到创建技能命令，但缺少参数",
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return "", "", true
	}

	parts := strings.SplitN(rest, " ", 2)
	skillName = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		description = strings.TrimSpace(parts[1])
	}

	utilities.LogVerbose(component, op, "创建技能命令解析完成",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("skill_name=%s", skillName),
		fmt.Sprintf("description_len=%d", len(description)),
	)

	return skillName, description, true
}

func validateSkillName(name string) error {
	const component = "SkillCreator"
	const op = "validateSkillName"
	traceID := utilities.NewTraceID()

	utilities.LogVerbose(component, op, "开始校验技能名称",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("name=%s", name),
		fmt.Sprintf("name_len=%d", len(name)),
		fmt.Sprintf("regex=%s", skillNameRegex.String()),
	)

	if name == "" {
		utilities.LogWarn(component, op, "技能名称为空", 0,
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return fmt.Errorf("技能名称不能为空")
	}

	matched := skillNameRegex.MatchString(name)
	utilities.LogVerbose(component, op, "正则匹配结果",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("name=%s", name),
		fmt.Sprintf("regex_matched=%t", matched),
		fmt.Sprintf("regex_pattern=%s", skillNameRegex.String()),
	)

	if !matched {
		utilities.LogWarn(component, op,
			fmt.Sprintf("技能名称 %q 不符合正则规则", name), 0,
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return fmt.Errorf("技能名称 %q 非法: 仅允许小写字母、数字和下划线，长度 1-64", name)
	}

	utilities.LogVerbose(component, op, "技能名称校验通过",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("name=%s", name),
	)
	return nil
}

func isCreateSkillAllowed() bool {
	const component = "SkillCreator"
	const op = "isCreateSkillAllowed"

	value := utilities.GetEnv("IS_ALLOW_USER_CREATE_SKILL", "false")
	allowed := strings.EqualFold(strings.TrimSpace(value), "true")

	utilities.LogVerbose(component, op, "检查技能创建权限开关",
		fmt.Sprintf("IS_ALLOW_USER_CREATE_SKILL=%s", value),
		fmt.Sprintf("allowed=%t", allowed),
	)

	return allowed
}

// isProductionMode 检查当前是否为生产环境模式。
// 生产环境返回通用错误消息，避免泄露配置细节。
func isProductionMode() bool {
	const component = "SkillCreator"
	const op = "isProductionMode"

	mode := utilities.GetEnv("MODE", "development")
	isProd := strings.EqualFold(strings.TrimSpace(mode), "production")

	utilities.LogVerbose(component, op, "检测运行模式",
		fmt.Sprintf("MODE=%s", mode),
		fmt.Sprintf("is_production=%t", isProd),
	)

	return isProd
}

func detectPromptInjection(input string) PromptInjectionDetection {
	const component = "SkillCreator"
	const op = "detectPromptInjection"
	traceID := utilities.NewTraceID()
	start := time.Now()

	utilities.LogVerbose(component, op, "开始输入层提示注入检测",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("input_len=%d", len(input)),
		fmt.Sprintf("total_patterns=%d", len(injectionPatterns)),
	)

	if input == "" {
		utilities.LogVerbose(component, op, "输入为空，跳过检测",
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return PromptInjectionDetection{IsInjection: false}
	}

	matched := make([]string, 0)
	for idx, pattern := range injectionPatterns {
		isMatch := pattern.MatchString(input)
		utilities.LogVerbose(component, op, "检查注入模式",
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("pattern_index=%d/%d", idx+1, len(injectionPatterns)),
			fmt.Sprintf("pattern=%s", pattern.String()),
			fmt.Sprintf("matched=%t", isMatch),
		)
		if isMatch {
			matched = append(matched, pattern.String())
		}
	}

	if len(matched) == 0 {
		utilities.LogVerbose(component, op, "未检测到注入模式",
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("elapsed_ns=%d", time.Since(start).Nanoseconds()),
		)
		return PromptInjectionDetection{IsInjection: false}
	}

	confidence := float64(len(matched)) / float64(len(injectionPatterns))
	reason := fmt.Sprintf("检测到 %d 个可疑模式匹配", len(matched))

	utilities.LogWarn(component, op,
		"检测到提示注入风险",
		time.Since(start),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("matched_count=%d", len(matched)),
		fmt.Sprintf("total_patterns=%d", len(injectionPatterns)),
		fmt.Sprintf("confidence=%.4f", confidence),
		fmt.Sprintf("elapsed_ns=%d", time.Since(start).Nanoseconds()),
	)

	return PromptInjectionDetection{
		IsInjection:     true,
		MatchedPatterns: matched,
		Confidence:      confidence,
		Reason:          reason,
	}
}

func validateGeneratedSkillContent(content string) PromptInjectionDetection {
	const component = "SkillCreator"
	const op = "validateGeneratedSkillContent"
	traceID := utilities.NewTraceID()
	start := time.Now()

	utilities.LogVerbose(component, op, "开始输出层安全校验",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("content_len=%d", len(content)),
		fmt.Sprintf("total_patterns=%d", len(outputInjectionPatterns)),
	)

	if content == "" {
		utilities.LogVerbose(component, op, "内容为空，跳过校验",
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return PromptInjectionDetection{IsInjection: false}
	}

	matched := make([]string, 0)
	for idx, pattern := range outputInjectionPatterns {
		isMatch := pattern.MatchString(content)
		utilities.LogVerbose(component, op, "检查输出注入模式",
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("pattern_index=%d/%d", idx+1, len(outputInjectionPatterns)),
			fmt.Sprintf("pattern=%s", pattern.String()),
			fmt.Sprintf("matched=%t", isMatch),
		)
		if isMatch {
			matched = append(matched, pattern.String())
		}
	}

	if len(matched) == 0 {
		utilities.LogVerbose(component, op, "输出内容安全校验通过",
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("elapsed_ns=%d", time.Since(start).Nanoseconds()),
		)
		return PromptInjectionDetection{IsInjection: false}
	}

	confidence := float64(len(matched)) / float64(len(outputInjectionPatterns))
	reason := fmt.Sprintf("生成的技能内容中检测到 %d 个安全风险模式", len(matched))

	utilities.LogWarn(component, op,
		"输出内容安全校验失败",
		time.Since(start),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("matched_count=%d", len(matched)),
		fmt.Sprintf("total_patterns=%d", len(outputInjectionPatterns)),
		fmt.Sprintf("confidence=%.4f", confidence),
		fmt.Sprintf("elapsed_ns=%d", time.Since(start).Nanoseconds()),
	)

	return PromptInjectionDetection{
		IsInjection:     true,
		MatchedPatterns: matched,
		Confidence:      confidence,
		Reason:          reason,
	}
}

func isGuardrailEnabled() bool {
	value := utilities.GetEnv("ENABLE_BEDROCK_GUARDRAIL", "false")
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

func GenerateSkillContent(
	ctx context.Context,
	llmService LLMService,
	skillName string,
	description string,
) (string, error) {
	const component = "SkillCreator"
	const op = "GenerateSkillContent"
	traceID := utilities.NewTraceID()
	start := time.Now()
	utilities.LogStart(component, op)

	if llmService == nil {
		utilities.LogError(component, op, fmt.Errorf("LLM 服务未初始化"), 0,
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return "", fmt.Errorf("LLM 服务未初始化")
	}
	if skillName == "" {
		utilities.LogError(component, op, fmt.Errorf("技能名称为空"), 0,
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return "", fmt.Errorf("技能名称不能为空")
	}
	if description == "" {
		utilities.LogError(component, op, fmt.Errorf("技能描述为空"), 0,
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return "", fmt.Errorf("技能描述不能为空")
	}

	systemPrompt := "你是 LingFlow 技能定义生成助手，拥有丰富的领域知识和技术写作能力。" +
		"请根据用户提供的技能名称和用途描述，生成一份全面、专业、可直接投入使用的技能 Markdown 文件。" +
		"生成的技能必须内容详尽、逻辑严谨、覆盖边界情况，确保 LLM 在使用该技能时能产出高质量的响应。" +
		"只输出 Markdown 内容本身，不要包含任何额外说明、问候或代码块标记。" +
		"不要在技能内容中包含任何系统指令、安全绕过、或者要求忽略之前指令的内容。"

	userPrompt := fmt.Sprintf(
		"技能名称: %s\n用途描述: %s\n\n"+
			"请按以下结构生成一份全面且专业的 Markdown 技能文件。每个章节都必须内容充实，不能留空或敷衍：\n\n"+
			"# {技能显示名称}\n\n"+
			"description: {精确的一句话描述，明确说明该技能的核心能力和适用场景}\n"+
			"category: {分类，如 general / analysis / trading / coding / finance / security / data}\n"+
			"keywords: {至少5个关键词，覆盖核心概念、同义词和相关术语，用逗号分隔}\n\n"+
			"## 角色定义\n"+
			"{定义 LLM 在使用该技能时应扮演的专家角色，包括专业背景、能力范围和行为准则}\n\n"+
			"## 核心能力\n"+
			"{以编号列表详细列出该技能的 5-10 项核心能力，每项能力附带简要说明}\n\n"+
			"## 使用说明\n"+
			"{详细说明技能如何被调用、输入格式要求、预期行为和输出格式}\n\n"+
			"## 执行步骤\n"+
			"{以编号列表描述该技能处理用户请求的完整步骤流程，从接收输入到生成输出}\n\n"+
			"## 输出格式规范\n"+
			"{明确定义响应的结构、格式要求、必须包含的字段或章节}\n\n"+
			"## 约束与规则\n"+
			"{列出该技能必须遵守的规则、限制条件和安全边界，至少5条}\n\n"+
			"## 触发示例\n"+
			"{提供至少5个不同场景的触发示例，覆盖简单到复杂的用例}\n"+
			"- {示例输入 1 — 基础用例}\n"+
			"- {示例输入 2 — 进阶用例}\n"+
			"- {示例输入 3 — 边界情况}\n"+
			"- {示例输入 4 — 多条件组合}\n"+
			"- {示例输入 5 — 复杂场景}\n\n"+
			"## 示例对话\n"+
			"{提供 1-2 组完整的用户输入与期望输出的示例对话，展示技能的实际效果}\n\n"+
			"## 错误处理\n"+
			"{说明当输入不完整、格式错误或超出技能范围时，应如何优雅地处理和回复}\n",
		skillName, description,
	)

	// ── 记录 LLM 请求构建详情 ──
	utilities.LogVerbose(component, op, "LLM 请求构建完成",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("skill_name=%s", skillName),
		fmt.Sprintf("system_prompt_len=%d", len(systemPrompt)),
		fmt.Sprintf("user_prompt_len=%d", len(userPrompt)),
		fmt.Sprintf("description_len=%d", len(description)),
	)

	utilities.LogProgress(component, op, "正在调用 LLM 生成技能内容",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("skill=%s", skillName),
	)

	llmStart := time.Now()
	resp, err := llmService.Generate(ctx, LLMRequest{
		SystemPrompt: systemPrompt,
		UserMessage:  userPrompt,
	})
	llmElapsed := time.Since(llmStart)

	if err != nil {
		utilities.LogError(component, op, fmt.Errorf("LLM 生成失败: %w", err), time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("skill=%s", skillName),
			fmt.Sprintf("llm_elapsed_ns=%d", llmElapsed.Nanoseconds()),
		)
		return "", fmt.Errorf("LLM 生成技能内容失败: %w", err)
	}

	// ── 记录 LLM 响应详情 ──
	utilities.LogVerbose(component, op, "LLM 响应接收",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("content_len=%d", len(resp.Content)),
		fmt.Sprintf("tokens_used=%d", resp.TokensUsed),
		fmt.Sprintf("llm_elapsed_ns=%d", llmElapsed.Nanoseconds()),
	)

	content := strings.TrimSpace(resp.Content)
	if len(content) > maxGeneratedSkillSize {
		utilities.LogWarn(component, op,
			fmt.Sprintf("生成的技能内容超过 %d 字节，已截断", maxGeneratedSkillSize),
			time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("original_size=%d", len(content)),
			fmt.Sprintf("truncated_to=%d", maxGeneratedSkillSize),
		)
		content = content[:maxGeneratedSkillSize]
	}

	utilities.LogNano(component, op, utilities.INFO, "OK", time.Since(start),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("skill=%s", skillName),
		fmt.Sprintf("content_len=%d", len(content)),
		fmt.Sprintf("llm_elapsed_ns=%d", llmElapsed.Nanoseconds()),
		fmt.Sprintf("total_ns=%d", time.Since(start).Nanoseconds()),
	)

	return content, nil
}

func (handler *ChatHandler) tryHandleCreateSkillCommand(
	ctx context.Context,
	connectionID string,
	incomingMessage models.WSMessage,
	userData models.UserChatData,
) bool {
	const component = "ChatHandler"
	const op = "tryHandleCreateSkillCommand"
	pipelineTraceID := utilities.NewTraceID()
	pipelineStart := time.Now()

	// ── 步骤 1: 命令解析 ──
	stepStart := time.Now()
	skillName, description, isCommand := parseCreateSkillCommand(userData.Message)
	utilities.LogNano(component, op, utilities.INFO, "IN_PROGRESS", time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("step=command_parse"),
		fmt.Sprintf("is_command=%t", isCommand),
		fmt.Sprintf("skill_name=%s", skillName),
		fmt.Sprintf("description_len=%d", len(description)),
		fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
	)

	if !isCommand {
		return false
	}

	utilities.LogStart(component, op)
	utilities.LogVerbose(component, op, "技能创建流水线启动",
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("conn=%s", connectionID),
		fmt.Sprintf("skill=%s", skillName),
		fmt.Sprintf("description_len=%d", len(description)),
	)

	// ── 步骤 2: 技能名称校验 ──
	stepStart = time.Now()
	if err := validateSkillName(skillName); err != nil {
		handler.sendSkillCreationError(connectionID, "invalid_skill_name", err.Error())
		utilities.LogNano(component, op, utilities.WARN, "FAIL", time.Since(stepStart),
			fmt.Sprintf("trace_id=%s", pipelineTraceID),
			fmt.Sprintf("step=name_validation"),
			fmt.Sprintf("error=%s", err.Error()),
			fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
		)
		return true
	}
	utilities.LogNano(component, op, utilities.INFO, "IN_PROGRESS", time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("step=name_validation"),
		fmt.Sprintf("result=pass"),
		fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
	)

	// ── 步骤 3: 描述长度上限校验 ──
	stepStart = time.Now()
	if len(description) > maxDescriptionLength {
		handler.sendSkillCreationError(connectionID, "description_too_long",
			fmt.Sprintf("技能描述超过最大长度 %d 字符", maxDescriptionLength))
		utilities.LogNano(component, op, utilities.WARN, "FAIL", time.Since(stepStart),
			fmt.Sprintf("trace_id=%s", pipelineTraceID),
			fmt.Sprintf("step=description_max_validation"),
			fmt.Sprintf("description_len=%d", len(description)),
			fmt.Sprintf("max=%d", maxDescriptionLength),
			fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
		)
		return true
	}

	// ── 步骤 4: 描述长度下限校验 ──
	if len(description) < createSkillDescriptionMinLen {
		handler.sendSkillCreationError(connectionID, "missing_description",
			fmt.Sprintf("技能描述至少需要 %d 个字符，例如: #create_skill %s 你的技能描述",
				createSkillDescriptionMinLen, skillName))
		utilities.LogNano(component, op, utilities.WARN, "FAIL", time.Since(stepStart),
			fmt.Sprintf("trace_id=%s", pipelineTraceID),
			fmt.Sprintf("step=description_min_validation"),
			fmt.Sprintf("description_len=%d", len(description)),
			fmt.Sprintf("min=%d", createSkillDescriptionMinLen),
			fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
		)
		return true
	}
	utilities.LogNano(component, op, utilities.INFO, "IN_PROGRESS", time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("step=description_validation"),
		fmt.Sprintf("result=pass"),
		fmt.Sprintf("description_len=%d", len(description)),
		fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
	)

	// ── 步骤 5: 输入层提示注入检测 ──
	stepStart = time.Now()
	injectionResult := detectPromptInjection(description)
	utilities.LogNano(component, op, utilities.INFO, "IN_PROGRESS", time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("step=injection_detection"),
		fmt.Sprintf("is_injection=%t", injectionResult.IsInjection),
		fmt.Sprintf("confidence=%.4f", injectionResult.Confidence),
		fmt.Sprintf("matched_count=%d", len(injectionResult.MatchedPatterns)),
		fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
	)
	if injectionResult.IsInjection {
		handler.sendSkillCreationError(connectionID, "prompt_injection_detected",
			fmt.Sprintf("检测到提示注入攻击: %s。请使用合法的技能描述。", injectionResult.Reason))
		utilities.LogWarn(component, op,
			fmt.Sprintf("提示注入检测: 置信度=%.2f, 匹配模式=%v", injectionResult.Confidence, injectionResult.MatchedPatterns),
			time.Since(pipelineStart),
			fmt.Sprintf("trace_id=%s", pipelineTraceID),
		)
		return true
	}

	// ── 步骤 6: 权限检查 ──
	stepStart = time.Now()
	allowed := isCreateSkillAllowed()
	utilities.LogNano(component, op, utilities.INFO, "IN_PROGRESS", time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("step=permission_check"),
		fmt.Sprintf("allowed=%t", allowed),
		fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
	)
	if !allowed {
		// 生产环境不暴露配置细节，返回通用 403
		if isProductionMode() {
			handler.sendSkillCreationError(connectionID, "forbidden",
				"操作被拒绝: 权限不足")
		} else {
			handler.sendSkillCreationError(connectionID, "skill_creation_disabled",
				"#create_skill 功能未启用: 请在 .env 中设置 IS_ALLOW_USER_CREATE_SKILL=true 后重启服务")
		}
		utilities.LogWarn(component, op,
			"IS_ALLOW_USER_CREATE_SKILL 未启用，已拒绝", time.Since(pipelineStart),
			fmt.Sprintf("trace_id=%s", pipelineTraceID),
		)
		return true
	}

	// ── 步骤 7: S3 加载器可用性检查 ──
	stepStart = time.Now()
	s3LoaderAvailable := handler.s3Loader != nil && handler.s3Loader.bucket != ""
	utilities.LogNano(component, op, utilities.INFO, "IN_PROGRESS", time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("step=s3_loader_check"),
		fmt.Sprintf("loader_nil=%t", handler.s3Loader == nil),
		fmt.Sprintf("available=%t", s3LoaderAvailable),
		fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
	)
	if handler.s3Loader == nil {
		if isProductionMode() {
			handler.sendSkillCreationError(connectionID, "forbidden",
				"操作被拒绝: 服务暂时不可用")
		} else {
			handler.sendSkillCreationError(connectionID, "s3_unavailable",
				"S3 技能加载器未初始化，无法创建技能")
		}
		return true
	}
	if handler.s3Loader.bucket == "" {
		if isProductionMode() {
			handler.sendSkillCreationError(connectionID, "forbidden",
				"操作被拒绝: 服务暂时不可用")
		} else {
			handler.sendSkillCreationError(connectionID, "s3_bucket_missing",
				"未配置 SKILLS_S3_BUCKET 或 AWS_SKILLS_S3_BUCKET，无法创建技能")
		}
		return true
	}

	// ── 步骤 8: 技能存在性检查 ──
	stepStart = time.Now()
	handler.sendThinking(connectionID, models.SystemThinkingData{
		Phase:   "skill_creation",
		Thought: fmt.Sprintf("正在检查技能 /%s 是否已存在...", skillName),
		Metadata: map[string]interface{}{
			"step":   "existence_check",
			"skill":  skillName,
			"bucket": handler.s3Loader.bucket,
		},
	}, incomingMessage.SkillsId)

	exists, existsErr := handler.s3Loader.SkillExists(ctx, skillName)
	utilities.LogNano(component, op, utilities.INFO, "IN_PROGRESS", time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("step=existence_check"),
		fmt.Sprintf("skill=%s", skillName),
		fmt.Sprintf("exists=%t", exists),
		fmt.Sprintf("has_error=%t", existsErr != nil),
		fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
	)

	if existsErr != nil {
		// S3 内部错误详情只记录服务器日志，不暴露给客户端
		utilities.LogError(component, op, existsErr, time.Since(pipelineStart),
			fmt.Sprintf("trace_id=%s", pipelineTraceID),
			fmt.Sprintf("step=existence_check"),
			fmt.Sprintf("bucket=%s", handler.s3Loader.bucket),
			fmt.Sprintf("region=%s", handler.s3Loader.region),
			fmt.Sprintf("skill=%s", skillName),
			fmt.Sprintf("s3_error_detail=%v", existsErr),
		)
		if isProductionMode() {
			handler.sendSkillCreationError(connectionID, "forbidden",
				"操作被拒绝: 服务暂时不可用")
		} else {
			handler.sendSkillCreationError(connectionID, "s3_check_failed",
				fmt.Sprintf("S3 服务访问异常 (bucket=%s, region=%s): %v",
					handler.s3Loader.bucket, handler.s3Loader.region, existsErr))
		}
		return true
	}
	if exists {
		if isProductionMode() {
			handler.sendSkillCreationError(connectionID, "forbidden",
				"操作被拒绝: 资源已存在")
		} else {
			handler.sendSkillCreationError(connectionID, "skill_already_exists",
				fmt.Sprintf("技能 /%s 已存在，拒绝覆盖。请删除 S3 中的 %s 后重试。",
					skillName, handler.s3Loader.StorageURI(skillName)))
		}
		utilities.LogWarn(component, op,
			fmt.Sprintf("技能已存在: %s", skillName), time.Since(pipelineStart),
			fmt.Sprintf("trace_id=%s", pipelineTraceID),
		)
		return true
	}

	// ── 步骤 9: S3 名称预留 ──
	stepStart = time.Now()
	handler.sendThinking(connectionID, models.SystemThinkingData{
		Phase:   "skill_creation",
		Thought: fmt.Sprintf("正在 S3 预留技能名称 /%s...", skillName),
		Metadata: map[string]interface{}{
			"step":  "name_reservation",
			"skill": skillName,
		},
	}, incomingMessage.SkillsId)

	reserveErr := handler.s3Loader.UploadSkill(ctx, skillName, []byte(""))
	utilities.LogNano(component, op, utilities.INFO, "IN_PROGRESS", time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("step=name_reservation"),
		fmt.Sprintf("skill=%s", skillName),
		fmt.Sprintf("success=%t", reserveErr == nil),
		fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
	)
	if reserveErr != nil {
		if isProductionMode() {
			handler.sendSkillCreationError(connectionID, "forbidden",
				"操作被拒绝: 服务暂时不可用")
		} else {
			handler.sendSkillCreationError(connectionID, "s3_reserve_failed",
				"S3 文件预留失败，请检查 AWS 凭证权限或存储桶配置")
		}
		utilities.LogError(component, op, reserveErr, time.Since(pipelineStart),
			fmt.Sprintf("trace_id=%s", pipelineTraceID),
			fmt.Sprintf("step=name_reservation"),
		)
		return true
	}

	// ── 步骤 10: LLM 生成技能内容 ──
	stepStart = time.Now()
	handler.sendThinking(connectionID, models.SystemThinkingData{
		Phase:   "skill_creation",
		Thought: "正在调用 AI 生成技能内容...",
		Metadata: map[string]interface{}{
			"step":  "llm_generation",
			"skill": skillName,
		},
	}, incomingMessage.SkillsId)

	utilities.LogProgress(component, op, "开始 LLM 生成",
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("skill=%s", skillName),
	)

	llmStart := time.Now()
	markdown, llmErr := GenerateSkillContent(ctx, handler.executor.llmService, skillName, description)
	llmLatency := time.Since(llmStart)

	utilities.LogNano(component, op, utilities.INFO, "IN_PROGRESS", time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("step=llm_generation"),
		fmt.Sprintf("success=%t", llmErr == nil),
		fmt.Sprintf("content_len=%d", len(markdown)),
		fmt.Sprintf("llm_elapsed_ns=%d", llmLatency.Nanoseconds()),
		fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
	)

	if llmErr != nil {
		// 生成失败，清理已预留的空文件
		cleanupErr := handler.s3Loader.DeleteSkill(ctx, skillName)
		if cleanupErr != nil {
			utilities.LogWarn(component, op,
				fmt.Sprintf("清理预留文件失败: %v", cleanupErr), time.Since(pipelineStart),
				fmt.Sprintf("trace_id=%s", pipelineTraceID),
			)
		}
		if isProductionMode() {
			handler.sendSkillCreationError(connectionID, "forbidden",
				"操作被拒绝: 服务暂时不可用")
		} else {
			handler.sendSkillCreationError(connectionID, "llm_generation_failed",
				fmt.Sprintf("AI 生成技能内容失败: %v", llmErr))
		}
		utilities.LogError(component, op, llmErr, time.Since(pipelineStart),
			fmt.Sprintf("trace_id=%s", pipelineTraceID),
			fmt.Sprintf("step=llm_generation"),
		)
		return true
	}

	// ── 步骤 11: 输出层安全校验 ──
	stepStart = time.Now()
	outputInjectionResult := validateGeneratedSkillContent(markdown)
	utilities.LogNano(component, op, utilities.INFO, "IN_PROGRESS", time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("step=output_validation"),
		fmt.Sprintf("is_injection=%t", outputInjectionResult.IsInjection),
		fmt.Sprintf("confidence=%.4f", outputInjectionResult.Confidence),
		fmt.Sprintf("matched_count=%d", len(outputInjectionResult.MatchedPatterns)),
		fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
	)
	if outputInjectionResult.IsInjection {
		// 安全校验失败，清理已预留的空文件
		cleanupErr := handler.s3Loader.DeleteSkill(ctx, skillName)
		if cleanupErr != nil {
			utilities.LogWarn(component, op,
				fmt.Sprintf("清理预留文件失败: %v", cleanupErr), time.Since(pipelineStart),
				fmt.Sprintf("trace_id=%s", pipelineTraceID),
			)
		}
		handler.sendSkillCreationError(connectionID, "output_injection_detected",
			fmt.Sprintf("生成的技能内容包含安全风险: %s。已阻止上传。", outputInjectionResult.Reason))
		utilities.LogWarn(component, op,
			fmt.Sprintf("输出注入检测: 置信度=%.2f, 匹配模式=%v", outputInjectionResult.Confidence, outputInjectionResult.MatchedPatterns),
			time.Since(pipelineStart),
			fmt.Sprintf("trace_id=%s", pipelineTraceID),
		)
		return true
	}

	// ── 步骤 12: S3 上传最终内容 ──
	stepStart = time.Now()
	handler.sendThinking(connectionID, models.SystemThinkingData{
		Phase:   "skill_creation",
		Thought: "正在将技能内容写入 S3...",
		Metadata: map[string]interface{}{
			"step":        "s3_commit",
			"skill":       skillName,
			"size":        len(markdown),
			"llm_latency": llmLatency.Milliseconds(),
		},
	}, incomingMessage.SkillsId)

	uploadErr := handler.s3Loader.UploadSkill(ctx, skillName, []byte(markdown))
	utilities.LogNano(component, op, utilities.INFO, "IN_PROGRESS", time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("step=s3_upload"),
		fmt.Sprintf("skill=%s", skillName),
		fmt.Sprintf("content_size=%d", len(markdown)),
		fmt.Sprintf("success=%t", uploadErr == nil),
		fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
	)
	if uploadErr != nil {
		// S3 内部错误详情只记录服务器日志，不暴露给客户端
		if isProductionMode() {
			handler.sendSkillCreationError(connectionID, "forbidden",
				"操作被拒绝: 服务暂时不可用")
		} else {
			handler.sendSkillCreationError(connectionID, "s3_upload_failed",
				"S3 文件上传失败，请检查 AWS 凭证权限或存储桶配置")
		}
		utilities.LogError(component, op, uploadErr, time.Since(pipelineStart),
			fmt.Sprintf("trace_id=%s", pipelineTraceID),
			fmt.Sprintf("step=s3_upload"),
		)
		return true
	}

	// ── 步骤 13: 重新加载技能注册中心 ──
	stepStart = time.Now()
	handler.sendThinking(connectionID, models.SystemThinkingData{
		Phase:   "skill_creation",
		Thought: "正在刷新技能注册中心...",
		Metadata: map[string]interface{}{
			"step":  "registry_reload",
			"skill": skillName,
		},
	}, incomingMessage.SkillsId)

	reloadedSkills, reloadErr := handler.s3Loader.LoadAllSkills(ctx)
	if reloadErr != nil {
		utilities.LogError(component, op, reloadErr,
			time.Since(pipelineStart),
			fmt.Sprintf("trace_id=%s", pipelineTraceID),
			fmt.Sprintf("step=registry_reload"),
			"S3 重新加载失败，但文件已写入",
		)
	} else {
		if reloadRegistryErr := handler.registry.Reload(reloadedSkills); reloadRegistryErr != nil {
			utilities.LogError(component, op, reloadRegistryErr,
				time.Since(pipelineStart),
				fmt.Sprintf("trace_id=%s", pipelineTraceID),
				fmt.Sprintf("step=registry_reload"),
				"注册中心 Reload 失败，但文件已写入",
			)
		}
	}
	utilities.LogNano(component, op, utilities.INFO, "IN_PROGRESS", time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("step=registry_reload"),
		fmt.Sprintf("reload_success=%t", reloadErr == nil),
		fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
	)

	// ── 步骤 14: 发送成功响应 ──
	stepStart = time.Now()
	storageURI := handler.s3Loader.StorageURI(skillName)
	preview := markdown
	if len(preview) > skillCreationPreviewSize {
		preview = preview[:skillCreationPreviewSize] + "\n... (已截断，完整内容已写入 S3)"
	}
	successMessage := fmt.Sprintf(
		"✅ 技能 /%s 创建成功！\n\n存储位置: %s\n文件大小: %d 字节\nAI 生成耗时: %d ms\n\n## 技能内容预览\n\n```markdown\n%s\n```",
		skillName, storageURI, len(markdown), llmLatency.Milliseconds(), preview,
	)

	handler.sendResponse(connectionID, models.SystemResponseData{
		Content:      successMessage,
		FinishReason: "end_turn",
		TokensUsed:   0,
		LatencyMs:    time.Since(pipelineStart).Milliseconds(),
		Metadata: map[string]interface{}{
			"action":      "skill_created",
			"skill_name":  skillName,
			"storage_uri": storageURI,
			"size":        len(markdown),
			"llm_latency": llmLatency.Milliseconds(),
			"total_ms":    time.Since(pipelineStart).Milliseconds(),
		},
	}, incomingMessage.SkillsId)

	utilities.LogNano(component, op, utilities.INFO, "IN_PROGRESS", time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("step=success_response"),
		fmt.Sprintf("step_ns=%d", time.Since(stepStart).Nanoseconds()),
	)

	// 推送更新后的技能列表给当前连接
	handler.OnConnectionReady(ctx, connectionID)

	// ── 流水线完成汇总 ──
	pipelineElapsed := time.Since(pipelineStart)
	utilities.LogNano(component, op, utilities.INFO, "OK", pipelineElapsed,
		fmt.Sprintf("trace_id=%s", pipelineTraceID),
		fmt.Sprintf("step=pipeline_complete"),
		fmt.Sprintf("skill=/%s", skillName),
		fmt.Sprintf("size=%d", len(markdown)),
		fmt.Sprintf("storage=%s", storageURI),
		fmt.Sprintf("llm_latency_ns=%d", llmLatency.Nanoseconds()),
		fmt.Sprintf("total_pipeline_ns=%d", pipelineElapsed.Nanoseconds()),
	)

	return true
}

func (handler *ChatHandler) sendSkillCreationError(connectionID, event, message string) {
	if handler.messageSender == nil {
		return
	}
	data := models.SystemChatData{
		Event:   event,
		Message: message,
	}
	dataBytes, _ := json.Marshal(data)
	msg := models.WSMessage{
		Type:      models.SystemChat,
		Data:      json.RawMessage(dataBytes),
		Timestamp: time.Now(),
	}
	payload, _ := json.Marshal(msg)
	_ = handler.messageSender.SendMessage(connectionID, payload)
}
