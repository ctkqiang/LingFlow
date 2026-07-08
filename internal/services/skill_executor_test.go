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
		Content:      "Mock response for: " + request.UserMessage,
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
			Instructions: fmt.Sprintf("Instructions for %s skill.", name),
			Rules:        []string{"Rule 1: Be accurate", "Rule 2: Be concise"},
		},
		SchemaVersion:       1,
		LastUpdatedTimestamp: time.Now(),
	}
}

// --- SkillRegistry Tests ---

func TestSkillRegistry_RegisterAndRetrieve(t *testing.T) {
	registry := NewSkillRegistry()

	skill := newTestSkillDefinition(
		"billing/refund", "Refund Status",
		"Check refund status and process refund requests",
		"billing", []string{"refund", "money", "payment", "return"},
	)

	if err := registry.RegisterSkill(skill); err != nil {
		t.Fatalf("RegisterSkill failed: %v", err)
	}

	if registry.SkillCount() != 1 {
		t.Fatalf("expected 1 skill, got %d", registry.SkillCount())
	}

	retrieved, exists := registry.GetSkill("billing/refund")
	if !exists {
		t.Fatal("expected skill to exist")
	}
	if retrieved.SkillDisplayName != "Refund Status" {
		t.Fatalf("expected display name 'Refund Status', got %q", retrieved.SkillDisplayName)
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
			name:    "empty identifier",
			skill:   models.SkillDefinition{SkillDisplayName: "Test"},
			wantErr: true,
		},
		{
			name:    "empty display name",
			skill:   models.SkillDefinition{SkillIdentifier: "test"},
			wantErr: true,
		},
		{
			name: "valid skill",
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
				t.Errorf("RegisterSkill() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSkillRegistry_UnregisterSkill(t *testing.T) {
	registry := NewSkillRegistry()

	skill := newTestSkillDefinition("test/remove", "Remove Me", "To be removed", "test", nil)
	_ = registry.RegisterSkill(skill)

	if !registry.UnregisterSkill("test/remove") {
		t.Fatal("expected UnregisterSkill to return true")
	}

	if registry.SkillCount() != 0 {
		t.Fatalf("expected 0 skills after unregister, got %d", registry.SkillCount())
	}

	if registry.UnregisterSkill("nonexistent") {
		t.Fatal("expected UnregisterSkill to return false for nonexistent skill")
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
			t.Fatalf("RegisterSkill failed: %v", err)
		}
	}

	results := registry.RetrieveSkills("I want a refund for my payment")
	if len(results) == 0 {
		t.Fatal("expected at least one result for refund query")
	}
	if results[0].Meta.SkillIdentifier != "billing/refund" {
		t.Fatalf("expected top result to be billing/refund, got %s", results[0].Meta.SkillIdentifier)
	}

	results = registry.RetrieveSkills("help me with my support ticket")
	if len(results) == 0 {
		t.Fatal("expected at least one result for support query")
	}
	if results[0].Meta.SkillIdentifier != "support/ticket" {
		t.Fatalf("expected top result to be support/ticket, got %s", results[0].Meta.SkillIdentifier)
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
		t.Fatal("expected a best skill match")
	}
	if best.SkillIdentifier != "billing/refund" {
		t.Fatalf("expected billing/refund, got %s", best.SkillIdentifier)
	}

	noMatch := registry.RetrieveBestSkill("xyzzy foobar baz")
	if noMatch != nil {
		t.Fatalf("expected no match for gibberish query, got %s", noMatch.SkillIdentifier)
	}
}

func TestSkillRegistry_ListSkills(t *testing.T) {
	registry := NewSkillRegistry()

	_ = registry.RegisterSkill(newTestSkillDefinition("a", "A", "desc a", "cat", nil))
	_ = registry.RegisterSkill(newTestSkillDefinition("b", "B", "desc b", "cat", nil))

	list := registry.ListSkills()
	if len(list) != 2 {
		t.Fatalf("expected 2 skills in list, got %d", len(list))
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
		t.Fatalf("Execute failed: %v", err)
	}

	if result.SkillUsed == nil {
		t.Fatal("expected a skill to be used")
	}
	if result.SkillUsed.SkillIdentifier != "billing/refund" {
		t.Fatalf("expected billing/refund skill, got %s", result.SkillUsed.SkillIdentifier)
	}

	if result.WSMessage.Type != models.SystemChat {
		t.Fatalf("expected SystemChat type, got %s", result.WSMessage.Type)
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
		t.Fatalf("Execute failed: %v", err)
	}

	if result.SkillUsed != nil {
		t.Fatal("expected no skill to be used for generic greeting")
	}

	if result.WSMessage.Type != models.SystemChat {
		t.Fatalf("expected SystemChat type, got %s", result.WSMessage.Type)
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
		t.Fatalf("ExecuteWithSkill failed: %v", err)
	}

	if result.SkillUsed == nil || result.SkillUsed.SkillIdentifier != "support/ticket" {
		t.Fatal("expected support/ticket skill to be used")
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
		t.Fatal("expected error for nonexistent skill")
	}

	execErr, ok := err.(*SkillExecutionError)
	if !ok {
		t.Fatalf("expected SkillExecutionError, got %T", err)
	}
	if execErr.Phase != "lookup" {
		t.Fatalf("expected phase 'lookup', got %q", execErr.Phase)
	}
}

func TestSkillExecutor_Execute_LLMFailure(t *testing.T) {
	registry := NewSkillRegistry()
	failingMock := &mockLLMService{
		generateFunc: func(ctx context.Context, request LLMRequest) (LLMResponse, error) {
			return LLMResponse{}, fmt.Errorf("LLM service unavailable")
		},
	}
	executor := NewSkillExecutor(registry, failingMock)

	_, err := executor.Execute(context.Background(), models.UserChatData{
		Message: "test message",
	})

	if err == nil {
		t.Fatal("expected error when LLM fails")
	}

	execErr, ok := err.(*SkillExecutionError)
	if !ok {
		t.Fatalf("expected SkillExecutionError, got %T", err)
	}
	if execErr.Phase != "generation" {
		t.Fatalf("expected phase 'generation', got %q", execErr.Phase)
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
		t.Fatalf("expected valid message, got error: %v", err)
	}
}

func TestValidateWSMessage_EmptyType(t *testing.T) {
	msg := models.WSMessage{
		Data:      json.RawMessage(`{}`),
		Timestamp: time.Now(),
	}

	if err := ValidateWSMessage(msg); err == nil {
		t.Fatal("expected error for empty type")
	}
}

func TestValidateWSMessage_InvalidType(t *testing.T) {
	msg := models.WSMessage{
		Type:      "invalid_type",
		Data:      json.RawMessage(`{}`),
		Timestamp: time.Now(),
	}

	if err := ValidateWSMessage(msg); err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestValidateWSMessage_EmptyData(t *testing.T) {
	msg := models.WSMessage{
		Type:      models.SystemChat,
		Timestamp: time.Now(),
	}

	if err := ValidateWSMessage(msg); err == nil {
		t.Fatal("expected error for empty data")
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
		t.Fatal("expected error for zero timestamp")
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
		t.Fatal("expected error for empty user message")
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
		t.Fatal("expected error for empty system event")
	}
}

// --- ChatHandler Tests ---

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
		t.Fatalf("HandleIncomingMessage failed: %v", err)
	}

	var response models.WSMessage
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response.Type != models.SystemChat {
		t.Fatalf("expected SystemChat response, got %s", response.Type)
	}

	var sysData models.SystemChatData
	if err := json.Unmarshal(response.Data, &sysData); err != nil {
		t.Fatalf("failed to unmarshal system data: %v", err)
	}

	if sysData.Event != "llm_response" {
		t.Fatalf("expected event 'llm_response', got %q", sysData.Event)
	}

	if sysData.Message == "" {
		t.Fatal("expected non-empty response message")
	}
}

func TestChatHandler_HandleSystemChat(t *testing.T) {
	mock := &mockLLMService{}
	handler := NewChatHandler(NewSkillRegistry(), mock)

	sysData := models.SystemChatData{
		Event:   "user_joined",
		Message: "A new user has joined",
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
		t.Fatalf("HandleIncomingMessage failed: %v", err)
	}

	var response models.WSMessage
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response.Type != models.SystemChat {
		t.Fatalf("expected SystemChat response, got %s", response.Type)
	}
}

func TestChatHandler_InvalidPayload(t *testing.T) {
	mock := &mockLLMService{}
	handler := NewChatHandler(NewSkillRegistry(), mock)

	responseBytes, err := handler.HandleIncomingMessage(context.Background(), []byte("not json"))
	if err != nil {
		t.Fatalf("expected error response in payload, not Go error: %v", err)
	}

	var response models.WSMessage
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}

	if response.Type != models.SystemChat {
		t.Fatalf("expected SystemChat error response, got %s", response.Type)
	}

	var sysData models.SystemChatData
	_ = json.Unmarshal(response.Data, &sysData)
	if sysData.Event != "parse_error" {
		t.Fatalf("expected parse_error event, got %q", sysData.Event)
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
		t.Fatal("expected non-empty context string")
	}

	if !containsSubstring(context, "Format Test") {
		t.Fatal("expected context to contain skill display name")
	}

	if !containsSubstring(context, "Instructions for Format Test skill") {
		t.Fatal("expected context to contain instructions")
	}

	if !containsSubstring(context, "Rule 1") {
		t.Fatal("expected context to contain rules")
	}
}

// --- 端到端管道测试 ---

func TestEndToEnd_SkillSelectionThroughResponseDelivery(t *testing.T) {
	// Step 1: Set up registry with multiple skills
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
			t.Fatalf("RegisterSkill failed: %v", err)
		}
	}

	// 步骤 2: 创建验证技能上下文注入的模拟 LLM
	var capturedRequest LLMRequest
	mock := &mockLLMService{
		generateFunc: func(ctx context.Context, request LLMRequest) (LLMResponse, error) {
			capturedRequest = request
			return LLMResponse{
				Content:      "Your refund has been processed successfully.",
				FinishReason: "stop",
				TokensUsed:   100,
				SkillID:      request.SkillID,
				Latency:      50 * time.Millisecond,
			}, nil
		},
	}

	// 步骤 3: 创建处理器并处理消息
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
		t.Fatalf("E2E: HandleIncomingMessage failed: %v", err)
	}

	// Step 4: Verify skill was selected and injected
	if capturedRequest.SkillID != "billing/refund" {
		t.Fatalf("E2E: expected skill billing/refund, got %q", capturedRequest.SkillID)
	}

	if capturedRequest.SkillContext == "" {
		t.Fatal("E2E: expected skill context to be injected into LLM request")
	}

	if !containsSubstring(capturedRequest.SkillContext, "Refund Status") {
		t.Fatal("E2E: expected skill context to contain skill name")
	}

	// Step 5: Verify response conforms to WSMessage structure
	var response models.WSMessage
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("E2E: failed to unmarshal response: %v", err)
	}

	if response.Type != models.SystemChat {
		t.Fatalf("E2E: expected SystemChat, got %s", response.Type)
	}

	if response.SkillsId != "billing/refund" {
		t.Fatalf("E2E: expected skills_id 'billing/refund', got %q", response.SkillsId)
	}

	// 步骤 6: 校验响应消息内容
	var sysData models.SystemChatData
	if err := json.Unmarshal(response.Data, &sysData); err != nil {
		t.Fatalf("E2E: failed to unmarshal system data: %v", err)
	}

	if sysData.Event != "llm_response" {
		t.Fatalf("E2E: expected event 'llm_response', got %q", sysData.Event)
	}

	if sysData.Message != "Your refund has been processed successfully." {
		t.Fatalf("E2E: unexpected response message: %q", sysData.Message)
	}

	// 步骤 7: 验证响应通过 WSMessage 校验
	if err := ValidateWSMessage(response); err != nil {
		t.Fatalf("E2E: response failed validation: %v", err)
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
