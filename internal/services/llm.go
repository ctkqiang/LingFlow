package services

import (
	"context"
	"fmt"
	"ling_flow/internal/models"
	"ling_flow/internal/utilities"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// BedrockConfig 保存 AWS Bedrock LLM 服务的运行时配置。
type BedrockConfig struct {
	Region      string        // AWS 区域
	ModelID     string        // Bedrock 模型标识符
	MaxTokens   int           // 响应最大 token 数
	Temperature float32       // 采样温度 0.0-1.0
	TopP        float32       // Top-p 核采样 0.0-1.0
	Timeout     time.Duration // 请求超时时长
}

// LLMRequest 表示发送给 LLM 后端的单次请求。
type LLMRequest struct {
	SystemPrompt string // 系统提示词
	UserMessage  string // 用户消息内容
	SkillContext string // 技能上下文（注入到系统提示词中）
	SkillID      string // 关联的技能标识符
}

// LLMResponse 表示从 LLM 后端解析后的响应结果。
type LLMResponse struct {
	Content      string        // 生成的文本内容
	FinishReason string        // 停止原因（如 end_turn、max_tokens 等）
	TokensUsed   int           // 总消耗 token 数（输入 + 输出）
	SkillID      string        // 关联的技能标识符
	Latency      time.Duration // 请求耗时
}

// LLMService 定义与 LLM 后端交互的接口。
type LLMService interface {
	// Generate 向 LLM 发送请求并返回生成结果。
	Generate(ctx context.Context, request LLMRequest) (LLMResponse, error)
	// HealthCheck 验证 LLM 后端是否可达。
	HealthCheck(ctx context.Context) error
}

// NewBedrockConfig 从环境变量加载 Bedrock 配置。
//
// 支持的环境变量：
//   - AWS_BEDROCK_REGION     : Bedrock 所在的 AWS 区域（默认: ap-east-1）
//   - AWS_BEDROCK_MODEL_ID   : 模型标识符（默认: anthropic.claude-3-5-sonnet-20241022-v2:0）
//   - AWS_BEDROCK_MAX_TOKENS  : 响应最大 token 数（默认: 2048）
//   - AWS_BEDROCK_TEMPERATURE : 采样温度 0.0-1.0（默认: 0.7）
//   - AWS_BEDROCK_TOP_P       : Top-p 核采样 0.0-1.0（默认: 0.9）
//   - AWS_BEDROCK_TIMEOUT     : 请求超时时长（默认: 60s）
func NewBedrockConfig() BedrockConfig {
	timeout, _ := time.ParseDuration(utilities.GetEnv("AWS_BEDROCK_TIMEOUT", "60s"))
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	maxTokens := 2048
	if envValue := utilities.GetEnv("AWS_BEDROCK_MAX_TOKENS", ""); envValue != "" {
		fmt.Sscanf(envValue, "%d", &maxTokens)
	}

	var temperature float32 = 0.7
	if envValue := utilities.GetEnv("AWS_BEDROCK_TEMPERATURE", ""); envValue != "" {
		fmt.Sscanf(envValue, "%f", &temperature)
	}

	var topP float32 = 0.9
	if envValue := utilities.GetEnv("AWS_BEDROCK_TOP_P", ""); envValue != "" {
		fmt.Sscanf(envValue, "%f", &topP)
	}

	return BedrockConfig{
		Region:      utilities.GetEnv("AWS_BEDROCK_REGION", utilities.AWSRegion("ap-east-1")),
		ModelID:     utilities.GetEnv("AWS_BEDROCK_MODEL_ID", "anthropic.claude-3-5-sonnet-20241022-v2:0"),
		MaxTokens:   maxTokens,
		Temperature: temperature,
		TopP:        topP,
		Timeout:     timeout,
	}
}

// BedrockLLMService 使用 AWS Bedrock Converse API 实现 LLMService 接口。
type BedrockLLMService struct {
	client *bedrockruntime.Client // Bedrock 运行时客户端
	config BedrockConfig          // 运行时配置
}

// NewBedrockLLMService 创建一个基于 Bedrock 的 LLM 服务实例。
// 使用 AWS 默认凭证链初始化客户端（IAM 角色 > 环境变量 > 共享凭证文件）。
func NewBedrockLLMService(ctx context.Context, bedrockConfig BedrockConfig) (*BedrockLLMService, error) {
	start := time.Now()
	utilities.LogStart("BedrockLLMService", "Init")

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(bedrockConfig.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("加载 Bedrock AWS 配置失败: %w", err)
	}

	client := bedrockruntime.NewFromConfig(awsCfg)

	utilities.LogSuccess("BedrockLLMService", "Init", time.Since(start),
		fmt.Sprintf("region=%s", bedrockConfig.Region),
		fmt.Sprintf("model=%s", bedrockConfig.ModelID),
	)

	return &BedrockLLMService{
		client: client,
		config: bedrockConfig,
	}, nil
}

// Generate 通过 AWS Bedrock Converse API 发送提示词并返回生成结果。
//
// Converse API 是 AWS 推荐的统一接口，适用于所有 Bedrock 模型。
// 它在内部自动处理不同模型提供商的请求/响应格式差异，
// 因此无需针对每个模型手动拼装 InvokeModel 的 JSON 载荷。
func (service *BedrockLLMService) Generate(ctx context.Context, request LLMRequest) (LLMResponse, error) {
	start := time.Now()
	utilities.LogStart("BedrockLLMService", "Generate")

	// 如果存在技能上下文，将其注入到系统提示词中
	systemPrompt := request.SystemPrompt
	if request.SkillContext != "" {
		systemPrompt = buildSkillAugmentedPrompt(systemPrompt, request.SkillContext)
	}

	// 构建 Converse API 请求输入
	converseInput := service.buildConverseInput(systemPrompt, request.UserMessage)

	// 设置超时上下文
	callCtx, cancel := context.WithTimeout(ctx, service.config.Timeout)
	defer cancel()

	// 调用 Bedrock Converse API
	output, err := service.client.Converse(callCtx, converseInput)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("Bedrock Converse API 调用失败: %w", err)
	}

	// 从响应中提取文本内容
	content, err := extractConverseTextContent(output)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("提取 Bedrock 响应内容失败: %w", err)
	}

	// 提取 token 使用量
	tokensUsed := 0
	if output.Usage != nil {
		tokensUsed = int(derefInt32(output.Usage.InputTokens)) + int(derefInt32(output.Usage.OutputTokens))
	}

	// 提取停止原因
	finishReason := string(output.StopReason)

	latency := time.Since(start)
	utilities.LogSuccess("BedrockLLMService", "Generate", latency,
		fmt.Sprintf("model=%s", service.config.ModelID),
		fmt.Sprintf("tokens_in=%d", safeTokenCount(output.Usage, true)),
		fmt.Sprintf("tokens_out=%d", safeTokenCount(output.Usage, false)),
		fmt.Sprintf("stop_reason=%s", finishReason),
	)

	return LLMResponse{
		Content:      content,
		FinishReason: finishReason,
		TokensUsed:   tokensUsed,
		SkillID:      request.SkillID,
		Latency:      latency,
	}, nil
}

// HealthCheck 通过发送一个最小请求来验证 Bedrock 服务是否可达。
func (service *BedrockLLMService) HealthCheck(ctx context.Context) error {
	start := time.Now()
	utilities.LogStart("BedrockLLMService", "HealthCheck")

	testRequest := LLMRequest{
		SystemPrompt: "You are a health check assistant.",
		UserMessage:  "Respond with OK.",
	}

	_, err := service.Generate(ctx, testRequest)
	if err != nil {
		utilities.LogError("BedrockLLMService", "HealthCheck", err, time.Since(start))
		return fmt.Errorf("Bedrock 健康检查失败: %w", err)
	}

	utilities.LogSuccess("BedrockLLMService", "HealthCheck", time.Since(start))
	return nil
}

// buildConverseInput 构建 Bedrock Converse API 的请求输入。
func (service *BedrockLLMService) buildConverseInput(
	systemPrompt string,
	userMessage string,
) *bedrockruntime.ConverseInput {
	maxTokens := int32(service.config.MaxTokens)

	converseInput := &bedrockruntime.ConverseInput{
		ModelId: &service.config.ModelID,
		Messages: []bedrocktypes.Message{
			{
				Role: bedrocktypes.ConversationRoleUser,
				Content: []bedrocktypes.ContentBlock{
					&bedrocktypes.ContentBlockMemberText{
						Value: userMessage,
					},
				},
			},
		},
		InferenceConfig: &bedrocktypes.InferenceConfiguration{
			MaxTokens:   &maxTokens,
			Temperature: &service.config.Temperature,
			TopP:        &service.config.TopP,
		},
	}

	// 通过 System 字段注入系统提示词
	if systemPrompt != "" {
		converseInput.System = []bedrocktypes.SystemContentBlock{
			&bedrocktypes.SystemContentBlockMemberText{
				Value: systemPrompt,
			},
		}
	}

	return converseInput
}

// extractConverseTextContent 从 Converse API 响应中提取文本内容。
func extractConverseTextContent(output *bedrockruntime.ConverseOutput) (string, error) {
	if output == nil {
		return "", fmt.Errorf("Bedrock 返回了空输出")
	}

	responseMessage, ok := output.Output.(*bedrocktypes.ConverseOutputMemberMessage)
	if !ok {
		return "", fmt.Errorf("Bedrock 输出类型不符合预期: %T", output.Output)
	}

	var textParts []string
	for _, contentBlock := range responseMessage.Value.Content {
		if textBlock, ok := contentBlock.(*bedrocktypes.ContentBlockMemberText); ok {
			textParts = append(textParts, textBlock.Value)
		}
	}

	if len(textParts) == 0 {
		return "", fmt.Errorf("Bedrock 响应中不包含文本内容")
	}

	return strings.Join(textParts, "\n"), nil
}

// safeTokenCount 安全地从 Bedrock 使用量元数据中提取 token 计数。
func safeTokenCount(usage *bedrocktypes.TokenUsage, isInput bool) int32 {
	if usage == nil {
		return 0
	}
	if isInput {
		return derefInt32(usage.InputTokens)
	}
	return derefInt32(usage.OutputTokens)
}

// derefInt32 安全地解引用 *int32 指针，若为 nil 则返回 0。
func derefInt32(pointer *int32) int32 {
	if pointer == nil {
		return 0
	}
	return *pointer
}

// MockLLMService 是一个用于开发和测试的模拟 LLM 服务，不调用真实的 Bedrock API。
// 通过设置环境变量 LLM_MOCK_MODE=true 启用。
type MockLLMService struct{}

// NewMockLLMService 创建一个模拟 LLM 服务实例。
func NewMockLLMService() *MockLLMService {
	utilities.LogProgress("LLM", "NewMockLLMService", "模拟 LLM 模式已启用，所有请求将返回模拟响应")
	return &MockLLMService{}
}

// Generate 返回一个模拟的 LLM 响应，包含用户消息和技能上下文的信息。
func (service *MockLLMService) Generate(ctx context.Context, request LLMRequest) (LLMResponse, error) {
	utilities.LogProgress("MockLLM", "Generate", fmt.Sprintf("收到请求: %s", truncateString(request.UserMessage, 50)))

	// 构建模拟响应内容
	var responseContent strings.Builder
	responseContent.WriteString("【模拟响应】\n\n")
	responseContent.WriteString(fmt.Sprintf("收到您的消息: \"%s\"\n\n", request.UserMessage))

	if request.SkillContext != "" {
		responseContent.WriteString("【已使用技能上下文】\n")
		responseContent.WriteString(truncateString(request.SkillContext, 200))
		responseContent.WriteString("\n\n")
	}

	if request.SystemPrompt != "" {
		responseContent.WriteString("【系统提示词已注入】\n")
		responseContent.WriteString(truncateString(request.SystemPrompt, 100))
		responseContent.WriteString("\n\n")
	}

	responseContent.WriteString("---\n")
	responseContent.WriteString("这是模拟响应。要获取真实 LLM 响应，请设置 LLM_MOCK_MODE=false 并配置正确的 AWS Bedrock 凭证。")

	return LLMResponse{
		Content:      responseContent.String(),
		FinishReason: "end_turn",
		TokensUsed:   len(request.UserMessage) * 2,
		SkillID:      request.SkillID,
		Latency:      100 * time.Millisecond,
	}, nil
}

// HealthCheck 模拟健康检查，始终返回成功。
func (service *MockLLMService) HealthCheck(ctx context.Context) error {
	utilities.LogProgress("MockLLM", "HealthCheck", "模拟健康检查通过")
	return nil
}

// NewLLMService 根据环境变量创建合适的 LLM 服务实例。
// 如果设置了 LLM_MOCK_MODE=true，返回 MockLLMService；否则返回 BedrockLLMService。
func NewLLMService(ctx context.Context) (LLMService, error) {
	if strings.EqualFold(utilities.GetEnv("LLM_MOCK_MODE", ""), "true") {
		return NewMockLLMService(), nil
	}
	return NewBedrockLLMService(ctx, NewBedrockConfig())
}

// truncateString 截断字符串，如果超过最大长度则添加省略号。
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// buildSkillAugmentedPrompt 将技能上下文注入到系统提示词中。
func buildSkillAugmentedPrompt(basePrompt, skillContext string) string {
	var promptBuilder strings.Builder
	promptBuilder.WriteString(basePrompt)
	promptBuilder.WriteString("\n\n")
	promptBuilder.WriteString("--- SKILL CONTEXT ---\n")
	promptBuilder.WriteString(skillContext)
	promptBuilder.WriteString("\n--- END SKILL CONTEXT ---")
	return promptBuilder.String()
}

// FormatSkillAsContext 将 SkillDefinition 转换为可注入提示词的上下文字符串。
func FormatSkillAsContext(skill models.SkillDefinition) string {
	var contextBuilder strings.Builder
	contextBuilder.WriteString(fmt.Sprintf("Skill: %s\n", skill.SkillDisplayName))
	contextBuilder.WriteString(fmt.Sprintf("Description: %s\n", skill.SkillDescription))

	if skill.MarkdownBody.Instructions != "" {
		contextBuilder.WriteString(fmt.Sprintf("\nInstructions:\n%s\n", skill.MarkdownBody.Instructions))
	}

	if len(skill.MarkdownBody.Rules) > 0 {
		contextBuilder.WriteString("\nRules:\n")
		for ruleIndex, rule := range skill.MarkdownBody.Rules {
			contextBuilder.WriteString(fmt.Sprintf("  %d. %s\n", ruleIndex+1, rule))
		}
	}

	return contextBuilder.String()
}
