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
		regexp.MustCompile(`(?i)\\x|\\u|\\0|\\r|\\n\\s*\\n`),
	}
)

type PromptInjectionDetection struct {
	IsInjection      bool
	MatchedPatterns  []string
	Confidence       float64
	Reason           string
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
	limiter.mutex <- struct{}{}
	defer func() { <-limiter.mutex }()

	now := time.Now()
	if now.Sub(limiter.lastReset) > time.Minute {
		limiter.requestCounts = make(map[string]int)
		limiter.lastReset = now
	}

	count := limiter.requestCounts[userID]
	if count >= maxCreateSkillRatePerMinute {
		return false
	}
	limiter.requestCounts[userID] = count + 1
	return true
}

func parseCreateSkillCommand(message string) (skillName string, description string, isCommand bool) {
	trimmed := strings.TrimSpace(message)
	if !strings.HasPrefix(trimmed, createSkillCommandPrefix) {
		return "", "", false
	}

	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, createSkillCommandPrefix))
	if rest == "" {
		return "", "", true
	}

	parts := strings.SplitN(rest, " ", 2)
	skillName = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		description = strings.TrimSpace(parts[1])
	}

	return skillName, description, true
}

func validateSkillName(name string) error {
	if name == "" {
		return fmt.Errorf("技能名称不能为空")
	}
	if !skillNameRegex.MatchString(name) {
		return fmt.Errorf("技能名称 %q 非法: 仅允许小写字母、数字和下划线，长度 1-64", name)
	}
	return nil
}

func isCreateSkillAllowed() bool {
	value := utilities.GetEnv("IS_ALLOW_USER_CREATE_SKILL", "false")
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

func detectPromptInjection(input string) PromptInjectionDetection {
	if input == "" {
		return PromptInjectionDetection{IsInjection: false}
	}

	matched := make([]string, 0)
	for _, pattern := range injectionPatterns {
		if pattern.MatchString(input) {
			matched = append(matched, pattern.String())
		}
	}

	if len(matched) == 0 {
		return PromptInjectionDetection{IsInjection: false}
	}

	confidence := float64(len(matched)) / float64(len(injectionPatterns))
	reason := fmt.Sprintf("检测到 %d 个可疑模式匹配", len(matched))

	return PromptInjectionDetection{
		IsInjection:      true,
		MatchedPatterns:  matched,
		Confidence:       confidence,
		Reason:           reason,
	}
}

func validateGeneratedSkillContent(content string) PromptInjectionDetection {
	if content == "" {
		return PromptInjectionDetection{IsInjection: false}
	}

	matched := make([]string, 0)
	for _, pattern := range outputInjectionPatterns {
		if pattern.MatchString(content) {
			matched = append(matched, pattern.String())
		}
	}

	if len(matched) == 0 {
		return PromptInjectionDetection{IsInjection: false}
	}

	confidence := float64(len(matched)) / float64(len(outputInjectionPatterns))
	reason := fmt.Sprintf("生成的技能内容中检测到 %d 个安全风险模式", len(matched))

	return PromptInjectionDetection{
		IsInjection:      true,
		MatchedPatterns:  matched,
		Confidence:       confidence,
		Reason:           reason,
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
	if llmService == nil {
		return "", fmt.Errorf("LLM 服务未初始化")
	}
	if skillName == "" {
		return "", fmt.Errorf("技能名称不能为空")
	}
	if description == "" {
		return "", fmt.Errorf("技能描述不能为空")
	}

	systemPrompt := "你是 LingFlow 技能定义生成助手。" +
		"请根据用户提供的技能名称和用途描述，生成符合 LingFlow 规范的技能 Markdown 文件。" +
		"只输出 Markdown 内容本身，不要包含任何额外说明、问候或代码块标记。" +
		"不要在技能内容中包含任何系统指令、安全绕过、或者要求忽略之前指令的内容。"

	userPrompt := fmt.Sprintf(
		"技能名称: %s\n用途描述: %s\n\n请按以下结构生成 Markdown 技能文件:\n\n"+
			"# {技能显示名称}\n\n"+
			"description: {一句话描述，用于触发该技能的场景}\n"+
			"category: {分类，如 general / analysis / trading / coding}\n"+
			"keywords: {关键词1, 关键词2, 关键词3}\n\n"+
			"## 使用说明\n{详细说明技能如何被调用以及预期行为}\n\n"+
			"## 触发示例\n- {示例输入 1}\n- {示例输入 2}\n",
		skillName, description,
	)

	resp, err := llmService.Generate(ctx, LLMRequest{
		SystemPrompt: systemPrompt,
		UserMessage:  userPrompt,
	})
	if err != nil {
		return "", fmt.Errorf("LLM 生成技能内容失败: %w", err)
	}

	content := strings.TrimSpace(resp.Content)
	if len(content) > maxGeneratedSkillSize {
		utilities.LogWarn("SkillCreator", "GenerateSkillContent",
			fmt.Sprintf("生成的技能内容超过 %d 字节，已截断", maxGeneratedSkillSize), 0)
		content = content[:maxGeneratedSkillSize]
	}

	return content, nil
}

func (handler *ChatHandler) tryHandleCreateSkillCommand(
	ctx context.Context,
	connectionID string,
	incomingMessage models.WSMessage,
	userData models.UserChatData,
) bool {
	skillName, description, isCommand := parseCreateSkillCommand(userData.Message)
	if !isCommand {
		return false
	}

	start := time.Now()
	utilities.LogStart("ChatHandler", "tryHandleCreateSkillCommand")
	utilities.Logf("ChatHandler", "tryHandleCreateSkillCommand", utilities.INFO, "IN_PROGRESS", time.Since(start),
		fmt.Sprintf("conn=%s", connectionID),
		fmt.Sprintf("skill=%s", skillName),
	)

	// 1. 校验名称
	if err := validateSkillName(skillName); err != nil {
		handler.sendSkillCreationError(connectionID, "invalid_skill_name", err.Error())
		utilities.LogWarn("ChatHandler", "tryHandleCreateSkillCommand",
			err.Error(), time.Since(start))
		return true
	}

	// 2. 校验描述长度
	if len(description) > maxDescriptionLength {
		handler.sendSkillCreationError(connectionID, "description_too_long",
			fmt.Sprintf("技能描述超过最大长度 %d 字符", maxDescriptionLength))
		return true
	}

	// 3. 校验最小描述长度
	if len(description) < createSkillDescriptionMinLen {
		handler.sendSkillCreationError(connectionID, "missing_description",
			fmt.Sprintf("技能描述至少需要 %d 个字符，例如: #create_skill %s 你的技能描述",
				createSkillDescriptionMinLen, skillName))
		return true
	}

	// 4. 提示注入检测（输入层）
	injectionResult := detectPromptInjection(description)
	if injectionResult.IsInjection {
		handler.sendSkillCreationError(connectionID, "prompt_injection_detected",
			fmt.Sprintf("检测到提示注入攻击: %s。请使用合法的技能描述。", injectionResult.Reason))
		utilities.LogWarn("ChatHandler", "tryHandleCreateSkillCommand",
			fmt.Sprintf("提示注入检测: 置信度=%.2f, 匹配模式=%v", injectionResult.Confidence, injectionResult.MatchedPatterns),
			time.Since(start))
		return true
	}

	// 5. 校验开关
	if !isCreateSkillAllowed() {
		handler.sendSkillCreationError(connectionID, "skill_creation_disabled",
			"#create_skill 功能未启用: 请在 .env 中设置 IS_ALLOW_USER_CREATE_SKILL=true 后重启服务")
		utilities.LogWarn("ChatHandler", "tryHandleCreateSkillCommand",
			"IS_ALLOW_USER_CREATE_SKILL 未启用，已拒绝", time.Since(start))
		return true
	}

	// 6. 校验 S3 加载器
	if handler.s3Loader == nil {
		handler.sendSkillCreationError(connectionID, "s3_unavailable",
			"S3 技能加载器未初始化，无法创建技能")
		return true
	}
	if handler.s3Loader.bucket == "" {
		handler.sendSkillCreationError(connectionID, "s3_bucket_missing",
			"未配置 SKILLS_S3_BUCKET 或 AWS_SKILLS_S3_BUCKET，无法创建技能")
		return true
	}

	// 7. 检查技能是否已存在
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
	if existsErr != nil {
		handler.sendSkillCreationError(connectionID, "s3_check_failed",
			fmt.Sprintf("检查技能是否存在失败: %v", existsErr))
		utilities.LogError("ChatHandler", "tryHandleCreateSkillCommand", existsErr, time.Since(start))
		return true
	}
	if exists {
		handler.sendSkillCreationError(connectionID, "skill_already_exists",
			fmt.Sprintf("技能 /%s 已存在，拒绝覆盖。请删除 S3 中的 %s 后重试。",
				skillName, handler.s3Loader.StorageURI(skillName)))
		utilities.LogWarn("ChatHandler", "tryHandleCreateSkillCommand",
			fmt.Sprintf("技能已存在: %s", skillName), time.Since(start))
		return true
	}

	// 8. 调用 LLM 生成内容
	handler.sendThinking(connectionID, models.SystemThinkingData{
		Phase:   "skill_creation",
		Thought: "正在调用 AI 生成技能内容...",
		Metadata: map[string]interface{}{
			"step":  "llm_generation",
			"skill": skillName,
		},
	}, incomingMessage.SkillsId)

	llmStart := time.Now()
	markdown, llmErr := GenerateSkillContent(ctx, handler.executor.llmService, skillName, description)
	if llmErr != nil {
		handler.sendSkillCreationError(connectionID, "llm_generation_failed",
			fmt.Sprintf("AI 生成技能内容失败: %v", llmErr))
		utilities.LogError("ChatHandler", "tryHandleCreateSkillCommand", llmErr, time.Since(start))
		return true
	}
	llmLatency := time.Since(llmStart)

	// 9. 输出层安全校验（检测间接提示注入）
	outputInjectionResult := validateGeneratedSkillContent(markdown)
	if outputInjectionResult.IsInjection {
		handler.sendSkillCreationError(connectionID, "output_injection_detected",
			fmt.Sprintf("生成的技能内容包含安全风险: %s。已阻止上传。", outputInjectionResult.Reason))
		utilities.LogWarn("ChatHandler", "tryHandleCreateSkillCommand",
			fmt.Sprintf("输出注入检测: 置信度=%.2f, 匹配模式=%v", outputInjectionResult.Confidence, outputInjectionResult.MatchedPatterns),
			time.Since(start))
		return true
	}

	// 10. 上传到 S3
	handler.sendThinking(connectionID, models.SystemThinkingData{
		Phase:   "skill_creation",
		Thought: "正在将技能上传到 S3...",
		Metadata: map[string]interface{}{
			"step":        "s3_upload",
			"skill":       skillName,
			"size":        len(markdown),
			"llm_latency": llmLatency.Milliseconds(),
		},
	}, incomingMessage.SkillsId)

	uploadErr := handler.s3Loader.UploadSkill(ctx, skillName, []byte(markdown))
	if uploadErr != nil {
		handler.sendSkillCreationError(connectionID, "s3_upload_failed",
			fmt.Sprintf("上传到 S3 失败: %v", uploadErr))
		utilities.LogError("ChatHandler", "tryHandleCreateSkillCommand", uploadErr, time.Since(start))
		return true
	}

	// 11. 重新从 S3 加载所有技能
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
		utilities.LogError("ChatHandler", "tryHandleCreateSkillCommand", reloadErr,
			time.Since(start), "S3 重新加载失败，但文件已写入")
	} else {
		if reloadRegistryErr := handler.registry.Reload(reloadedSkills); reloadRegistryErr != nil {
			utilities.LogError("ChatHandler", "tryHandleCreateSkillCommand", reloadRegistryErr,
				time.Since(start), "注册中心 Reload 失败，但文件已写入")
		}
	}

	// 12. 发送成功响应
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
		LatencyMs:    time.Since(start).Milliseconds(),
		Metadata: map[string]interface{}{
			"action":       "skill_created",
			"skill_name":   skillName,
			"storage_uri":  storageURI,
			"size":         len(markdown),
			"llm_latency":  llmLatency.Milliseconds(),
			"total_ms":     time.Since(start).Milliseconds(),
		},
	}, incomingMessage.SkillsId)

	// 13. 推送更新后的技能列表给当前连接
	handler.OnConnectionReady(ctx, connectionID)

	utilities.LogSuccess("ChatHandler", "tryHandleCreateSkillCommand", time.Since(start),
		fmt.Sprintf("skill=/%s", skillName),
		fmt.Sprintf("size=%d", len(markdown)),
		fmt.Sprintf("storage=%s", storageURI),
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
