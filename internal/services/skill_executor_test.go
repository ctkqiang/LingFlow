package services

import (
	"context"
	"encoding/json"
	"fmt"
	"ling_flow/internal/models"
	"testing"
	"time"
)

// mockLLMService 是 LLMService 的测试替身。
type mockLLMService struct {
	generateFunc func(ctx context.Context, request LLMRequest) (LLMResponse, error)
}

func (m *mockLLMService) Generate(ctx context.Context, request LLMRequest) (LLMResponse, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, request)
	}
	return LLMResponse{
		Content:      "模拟响应: " + request.UserMessage,
		FinishReason: "stop",
		TokensUsed:   42,
		SkillID:      request.SkillID,
		Latency:      10 * time.Millisecond,
	}, nil
}

func (m *mockLLMService) HealthCheck(ctx context.Context) error {
	return nil
}

func newTestSkillDefinition(id, name, description, category string, keywords []string) models.SkillDefinition {
	return models.SkillDefinition{
		SkillIdentifier:  id,
		SkillDisplayName: name,
		SkillDescription: description,
		SearchKeywords:   keywords,
		SkillCategory:    category,
		MarkdownBody: models.SkillsMarkdownBody{
			Instructions: fmt.Sprintf("%s 技能的使用说明。", name),
			Rules:        []string{"规则 1: 保持准确", "规则 2: 保持简洁"},
		},
		SchemaVersion:        1,
		LastUpdatedTimestamp: time.Now(),
	}
}

// --- 技能注册中心测试 ---

func TestSkillRegistry_RegisterAndRetrieve(t *testing.T) {
	registry := NewSkillRegistry()

	skill := newTestSkillDefinition(
		"billing/refund", "Refund Status",
		"Check refund status and process refund requests",
		"billing", []string{"refund", "money", "payment", "return"},
	)

	if err := registry.RegisterSkill(skill); err != nil {
		t.Fatalf("注册技能失败: %v", err)
	}

	if registry.SkillCount() != 1 {
		t.Fatalf("期望 1 个技能，实际得到 %d 个", registry.SkillCount())
	}

	retrieved, exists := registry.GetSkill("billing/refund")
	if !exists {
		t.Fatal("期望技能存在")
	}
	if retrieved.SkillDisplayName != "Refund Status" {
		t.Fatalf("期望显示名称为 'Refund Status'，实际得到 %q", retrieved.SkillDisplayName)
	}
}

func TestSkillRegistry_RegisterValidation(t *testing.T) {
	registry := NewSkillRegistry()

	tests := []struct {
		name    string
		skill   models.SkillDefinition
		wantErr bool
	}{
		{
			name:    "标识符为空",
			skill:   models.SkillDefinition{SkillDisplayName: "Test"},
			wantErr: true,
		},
		{
			name:    "显示名称为空",
			skill:   models.SkillDefinition{SkillIdentifier: "test"},
			wantErr: true,
		},
		{
			name: "有效技能",
			skill: newTestSkillDefinition(
				"test/valid", "Valid Skill", "A valid test skill", "test", nil,
			),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.RegisterSkill(tt.skill)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterSkill() 错误 = %v, 期望错误 %v", err, tt.wantErr)
			}
		})
	}
}

func TestSkillRegistry_UnregisterSkill(t *testing.T) {
	registry := NewSkillRegistry()

	skill := newTestSkillDefinition("test/remove", "Remove Me", "To be removed", "test", nil)
	_ = registry.RegisterSkill(skill)

	if !registry.UnregisterSkill("test/remove") {
		t.Fatal("期望 UnregisterSkill 返回 true")
	}

	if registry.SkillCount() != 0 {
		t.Fatalf("注销后期望 0 个技能，实际得到 %d 个", registry.SkillCount())
	}

	if registry.UnregisterSkill("nonexistent") {
		t.Fatal("期望对不存在的技能 UnregisterSkill 返回 false")
	}
}

func TestSkillRegistry_RetrieveSkills(t *testing.T) {
	registry := NewSkillRegistry()

	skills := []models.SkillDefinition{
		newTestSkillDefinition(
			"billing/refund", "Refund Status",
			"Check refund status and process refund requests for payments",
			"billing", []string{"refund", "money", "payment", "return"},
		),
		newTestSkillDefinition(
			"support/ticket", "Support Ticket",
			"Create and manage customer support tickets and issues",
			"support", []string{"ticket", "issue", "help", "problem"},
		),
		newTestSkillDefinition(
			"account/settings", "Account Settings",
			"Manage user account settings and preferences",
			"account", []string{"account", "settings", "profile", "preferences"},
		),
	}

	for _, s := range skills {
		if err := registry.RegisterSkill(s); err != nil {
			t.Fatalf("注册技能失败: %v", err)
		}
	}

	results := registry.RetrieveSkills("I want a refund for my payment")
	if len(results) == 0 {
		t.Fatal("退款查询期望至少返回一个结果")
	}
	if results[0].Meta.SkillIdentifier != "billing/refund" {
		t.Fatalf("期望排名第一的结果为 billing/refund，实际得到 %s", results[0].Meta.SkillIdentifier)
	}

	results = registry.RetrieveSkills("help me with my support ticket")
	if len(results) == 0 {
		t.Fatal("客服查询期望至少返回一个结果")
	}
	if results[0].Meta.SkillIdentifier != "support/ticket" {
		t.Fatalf("期望排名第一的结果为 support/ticket，实际得到 %s", results[0].Meta.SkillIdentifier)
	}
}

func TestSkillRegistry_RetrieveBestSkill(t *testing.T) {
	registry := NewSkillRegistry()

	skill := newTestSkillDefinition(
		"billing/refund", "Refund Status",
		"Check refund status and process refund requests",
		"billing", []string{"refund", "money", "payment"},
	)
	_ = registry.RegisterSkill(skill)

	best := registry.RetrieveBestSkill("refund my payment please")
	if best == nil {
		t.Fatal("期望匹配到最佳技能")
	}
	if best.SkillIdentifier != "billing/refund" {
		t.Fatalf("期望 billing/refund，实际得到 %s", best.SkillIdentifier)
	}

	noMatch := registry.RetrieveBestSkill("xyzzy foobar baz")
	if noMatch != nil {
		t.Fatalf("期望无意义查询不匹配任何技能，实际得到 %s", noMatch.SkillIdentifier)
	}
}

func TestSkillRegistry_ListSkills(t *testing.T) {
	registry := NewSkillRegistry()

	_ = registry.RegisterSkill(newTestSkillDefinition("a", "A", "desc a", "cat", nil))
	_ = registry.RegisterSkill(newTestSkillDefinition("b", "B", "desc b", "cat", nil))

	list := registry.ListSkills()
	if len(list) != 2 {
		t.Fatalf("期望列表中有 2 个技能，实际得到 %d 个", len(list))
	}
}

// --- 技能执行器测试 ---

func TestSkillExecutor_Execute_WithSkillMatch(t *testing.T) {
	registry := NewSkillRegistry()
	_ = registry.RegisterSkill(newTestSkillDefinition(
		"billing/refund", "Refund Status",
		"Check refund status and process refund requests",
		"billing", []string{"refund", "money", "payment"},
	))

	mock := &mockLLMService{}
	executor := NewSkillExecutor(registry, mock)

	result, err := executor.Execute(context.Background(), models.UserChatData{
		ID:      1,
		UserID:  "user-123",
		Message: "I need a refund for my payment",
	})
	if err != nil {
		t.Fatalf("Execute 执行失败: %v", err)
	}

	if result.SkillUsed == nil {
		t.Fatal("期望使用了某个技能")
	}
	if result.SkillUsed.SkillIdentifier != "billing/refund" {
		t.Fatalf("期望使用 billing/refund 技能，实际得到 %s", result.SkillUsed.SkillIdentifier)
	}

	if result.WSMessage.Type != models.SystemChat {
		t.Fatalf("期望 SystemChat 类型，实际得到 %s", result.WSMessage.Type)
	}
}

func TestSkillExecutor_Execute_WithoutSkillMatch(t *testing.T) {
	registry := NewSkillRegistry()
	mock := &mockLLMService{}
	executor := NewSkillExecutor(registry, mock)

	result, err := executor.Execute(context.Background(), models.UserChatData{
		ID:      2,
		UserID:  "user-456",
		Message: "Hello, how are you?",
	})
	if err != nil {
		t.Fatalf("Execute 执行失败: %v", err)
	}

	if result.SkillUsed != nil {
		t.Fatal("期望通用问候不使用任何技能")
	}

	if result.WSMessage.Type != models.SystemChat {
		t.Fatalf("期望 SystemChat 类型，实际得到 %s", result.WSMessage.Type)
	}
}

func TestSkillExecutor_ExecuteWithSkill_Explicit(t *testing.T) {
	registry := NewSkillRegistry()
	_ = registry.RegisterSkill(newTestSkillDefinition(
		"support/ticket", "Support Ticket",
		"Create and manage support tickets",
		"support", []string{"ticket", "support"},
	))

	mock := &mockLLMService{}
	executor := NewSkillExecutor(registry, mock)

	result, err := executor.ExecuteWithSkill(context.Background(), models.UserChatData{
		ID:      3,
		UserID:  "user-789",
		Message: "Create a ticket",
	}, "support/ticket")
	if err != nil {
		t.Fatalf("ExecuteWithSkill 执行失败: %v", err)
	}

	if result.SkillUsed == nil || result.SkillUsed.SkillIdentifier != "support/ticket" {
		t.Fatal("期望使用 support/ticket 技能")
	}
}

func TestSkillExecutor_ExecuteWithSkill_NotFound(t *testing.T) {
	registry := NewSkillRegistry()
	mock := &mockLLMService{}
	executor := NewSkillExecutor(registry, mock)

	_, err := executor.ExecuteWithSkill(context.Background(), models.UserChatData{
		Message: "test",
	}, "nonexistent/skill")

	if err == nil {
		t.Fatal("期望对不存在的技能返回错误")
	}

	execErr, ok := err.(*SkillExecutionError)
	if !ok {
		t.Fatalf("期望 SkillExecutionError 类型，实际得到 %T", err)
	}
	if execErr.Phase != "lookup" {
		t.Fatalf("期望阶段为 'lookup'，实际得到 %q", execErr.Phase)
	}
}

func TestSkillExecutor_Execute_LLMFailure(t *testing.T) {
	registry := NewSkillRegistry()
	failingMock := &mockLLMService{
		generateFunc: func(ctx context.Context, request LLMRequest) (LLMResponse, error) {
			return LLMResponse{}, fmt.Errorf("LLM 服务不可用")
		},
	}
	executor := NewSkillExecutor(registry, failingMock)

	_, err := executor.Execute(context.Background(), models.UserChatData{
		Message: "测试消息",
	})

	if err == nil {
		t.Fatal("期望 LLM 失败时返回错误")
	}

	execErr, ok := err.(*SkillExecutionError)
	if !ok {
		t.Fatalf("期望 SkillExecutionError 类型，实际得到 %T", err)
	}
	if execErr.Phase != "generation" {
		t.Fatalf("期望阶段为 'generation'，实际得到 %q", execErr.Phase)
	}
}

// --- WSMessage 校验测试 ---

func TestValidateWSMessage_Valid(t *testing.T) {
	chatData := models.SystemChatData{Event: "test", Message: "hello"}
	dataBytes, _ := json.Marshal(chatData)

	msg := models.WSMessage{
		Type:      models.SystemChat,
		Data:      json.RawMessage(dataBytes),
		Timestamp: time.Now(),
	}

	if err := ValidateWSMessage(msg); err != nil {
		t.Fatalf("期望消息有效，实际得到错误: %v", err)
	}
}

func TestValidateWSMessage_EmptyType(t *testing.T) {
	msg := models.WSMessage{
		Data:      json.RawMessage(`{}`),
		Timestamp: time.Now(),
	}

	if err := ValidateWSMessage(msg); err == nil {
		t.Fatal("期望类型为空时返回错误")
	}
}

func TestValidateWSMessage_InvalidType(t *testing.T) {
	msg := models.WSMessage{
		Type:      "invalid_type",
		Data:      json.RawMessage(`{}`),
		Timestamp: time.Now(),
	}

	if err := ValidateWSMessage(msg); err == nil {
		t.Fatal("期望无效类型时返回错误")
	}
}

func TestValidateWSMessage_EmptyData(t *testing.T) {
	msg := models.WSMessage{
		Type:      models.SystemChat,
		Timestamp: time.Now(),
	}

	if err := ValidateWSMessage(msg); err == nil {
		t.Fatal("期望数据为空时返回错误")
	}
}

func TestValidateWSMessage_ZeroTimestamp(t *testing.T) {
	chatData := models.SystemChatData{Event: "test", Message: "hello"}
	dataBytes, _ := json.Marshal(chatData)

	msg := models.WSMessage{
		Type: models.SystemChat,
		Data: json.RawMessage(dataBytes),
	}

	if err := ValidateWSMessage(msg); err == nil {
		t.Fatal("期望时间戳为零值时返回错误")
	}
}

func TestValidateWSMessage_UserChat_EmptyMessage(t *testing.T) {
	userData := models.UserChatData{ID: 1, UserID: "u1", Message: ""}
	dataBytes, _ := json.Marshal(userData)

	msg := models.WSMessage{
		Type:      models.UserChat,
		Data:      json.RawMessage(dataBytes),
		Timestamp: time.Now(),
	}

	if err := ValidateWSMessage(msg); err == nil {
		t.Fatal("期望用户消息为空时返回错误")
	}
}

func TestValidateWSMessage_SystemChat_EmptyEvent(t *testing.T) {
	sysData := models.SystemChatData{Event: "", Message: "hello"}
	dataBytes, _ := json.Marshal(sysData)

	msg := models.WSMessage{
		Type:      models.SystemChat,
		Data:      json.RawMessage(dataBytes),
		Timestamp: time.Now(),
	}

	if err := ValidateWSMessage(msg); err == nil {
		t.Fatal("期望系统事件为空时返回错误")
	}
}

// --- 聊天处理器测试 ---

func TestChatHandler_HandleUserChat(t *testing.T) {
	registry := NewSkillRegistry()
	_ = registry.RegisterSkill(newTestSkillDefinition(
		"billing/refund", "Refund Status",
		"Check refund status and process refund requests",
		"billing", []string{"refund", "payment"},
	))

	mock := &mockLLMService{}
	handler := NewChatHandler(registry, mock)

	userData := models.UserChatData{
		ID:      1,
		UserID:  "user-001",
		Message: "I want a refund",
	}
	dataBytes, _ := json.Marshal(userData)

	incoming := models.WSMessage{
		Type:      models.UserChat,
		Data:      json.RawMessage(dataBytes),
		Timestamp: time.Now(),
	}
	incomingBytes, _ := json.Marshal(incoming)

	responseBytes, err := handler.HandleIncomingMessage(context.Background(), incomingBytes)
	if err != nil {
		t.Fatalf("HandleIncomingMessage 处理失败: %v", err)
	}

	var response models.WSMessage
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("反序列化响应失败: %v", err)
	}

	if response.Type != models.SystemChat {
		t.Fatalf("期望 SystemChat 响应，实际得到 %s", response.Type)
	}

	var sysData models.SystemChatData
	if err := json.Unmarshal(response.Data, &sysData); err != nil {
		t.Fatalf("反序列化系统数据失败: %v", err)
	}

	if sysData.Event != "llm_response" {
		t.Fatalf("期望事件为 'llm_response'，实际得到 %q", sysData.Event)
	}

	if sysData.Message == "" {
		t.Fatal("期望响应消息非空")
	}
}

func TestChatHandler_HandleSystemChat(t *testing.T) {
	mock := &mockLLMService{}
	handler := NewChatHandler(NewSkillRegistry(), mock)

	sysData := models.SystemChatData{
		Event:   "user_joined",
		Message: "新用户已加入",
	}
	dataBytes, _ := json.Marshal(sysData)

	incoming := models.WSMessage{
		Type:      models.SystemChat,
		Data:      json.RawMessage(dataBytes),
		Timestamp: time.Now(),
	}
	incomingBytes, _ := json.Marshal(incoming)

	responseBytes, err := handler.HandleIncomingMessage(context.Background(), incomingBytes)
	if err != nil {
		t.Fatalf("HandleIncomingMessage 处理失败: %v", err)
	}

	var response models.WSMessage
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("反序列化响应失败: %v", err)
	}

	if response.Type != models.SystemChat {
		t.Fatalf("期望 SystemChat 响应，实际得到 %s", response.Type)
	}
}

func TestChatHandler_InvalidPayload(t *testing.T) {
	mock := &mockLLMService{}
	handler := NewChatHandler(NewSkillRegistry(), mock)

	responseBytes, err := handler.HandleIncomingMessage(context.Background(), []byte("not json"))
	if err != nil {
		t.Fatalf("期望错误在载荷中返回，而非 Go 错误: %v", err)
	}

	var response models.WSMessage
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("反序列化错误响应失败: %v", err)
	}

	if response.Type != models.SystemChat {
		t.Fatalf("期望 SystemChat 错误响应，实际得到 %s", response.Type)
	}

	var sysData models.SystemChatData
	_ = json.Unmarshal(response.Data, &sysData)
	if sysData.Event != "parse_error" {
		t.Fatalf("期望 parse_error 事件，实际得到 %q", sysData.Event)
	}
}

// --- 技能上下文格式化测试 ---

func TestFormatSkillAsContext(t *testing.T) {
	skill := newTestSkillDefinition(
		"test/format", "Format Test",
		"Test skill for formatting",
		"test", nil,
	)

	context := FormatSkillAsContext(skill)

	if context == "" {
		t.Fatal("期望上下文字符串非空")
	}

	if !containsSubstring(context, "Format Test") {
		t.Fatal("期望上下文包含技能显示名称")
	}

	if !containsSubstring(context, "Format Test 技能的使用说明") {
		t.Fatal("期望上下文包含使用说明")
	}

	if !containsSubstring(context, "规则 1") {
		t.Fatal("期望上下文包含规则")
	}
}

// --- 端到端管道测试 ---

func TestEndToEnd_SkillSelectionThroughResponseDelivery(t *testing.T) {
	registry := NewSkillRegistry()

	skills := []models.SkillDefinition{
		newTestSkillDefinition(
			"billing/refund", "Refund Status",
			"Check refund status and process refund requests for payments",
			"billing", []string{"refund", "money", "payment", "return"},
		),
		newTestSkillDefinition(
			"support/ticket", "Support Ticket",
			"Create and manage customer support tickets and issues",
			"support", []string{"ticket", "issue", "help", "problem"},
		),
	}

	for _, s := range skills {
		if err := registry.RegisterSkill(s); err != nil {
			t.Fatalf("注册技能失败: %v", err)
		}
	}

	var capturedRequest LLMRequest
	mock := &mockLLMService{
		generateFunc: func(ctx context.Context, request LLMRequest) (LLMResponse, error) {
			capturedRequest = request
			return LLMResponse{
				Content:      "您的退款已成功处理。",
				FinishReason: "stop",
				TokensUsed:   100,
				SkillID:      request.SkillID,
				Latency:      50 * time.Millisecond,
			}, nil
		},
	}

	handler := NewChatHandler(registry, mock)

	userData := models.UserChatData{
		ID:      1,
		UserID:  "user-e2e",
		Message: "I need a refund for my payment",
	}
	dataBytes, _ := json.Marshal(userData)

	incoming := models.WSMessage{
		Type:      models.UserChat,
		Data:      json.RawMessage(dataBytes),
		Timestamp: time.Now(),
	}
	incomingBytes, _ := json.Marshal(incoming)

	responseBytes, err := handler.HandleIncomingMessage(context.Background(), incomingBytes)
	if err != nil {
		t.Fatalf("端到端: HandleIncomingMessage 处理失败: %v", err)
	}

	if capturedRequest.SkillID != "billing/refund" {
		t.Fatalf("端到端: 期望技能 billing/refund，实际得到 %q", capturedRequest.SkillID)
	}

	if capturedRequest.SkillContext == "" {
		t.Fatal("端到端: 期望技能上下文已注入到 LLM 请求中")
	}

	if !containsSubstring(capturedRequest.SkillContext, "Refund Status") {
		t.Fatal("端到端: 期望技能上下文包含技能名称")
	}

	var response models.WSMessage
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("端到端: 反序列化响应失败: %v", err)
	}

	if response.Type != models.SystemChat {
		t.Fatalf("端到端: 期望 SystemChat，实际得到 %s", response.Type)
	}

	if response.SkillsId != "billing/refund" {
		t.Fatalf("端到端: 期望 skills_id 为 'billing/refund'，实际得到 %q", response.SkillsId)
	}

	var sysData models.SystemChatData
	if err := json.Unmarshal(response.Data, &sysData); err != nil {
		t.Fatalf("端到端: 反序列化系统数据失败: %v", err)
	}

	if sysData.Event != "llm_response" {
		t.Fatalf("端到端: 期望事件为 'llm_response'，实际得到 %q", sysData.Event)
	}

	if sysData.Message != "您的退款已成功处理。" {
		t.Fatalf("端到端: 响应消息不符合预期: %q", sysData.Message)
	}

	if err := ValidateWSMessage(response); err != nil {
		t.Fatalf("端到端: 响应未通过校验: %v", err)
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsCheck(s, substr))
}

func containsCheck(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
