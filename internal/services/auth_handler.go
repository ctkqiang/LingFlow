package services

import (
	"encoding/json"
	"ling_flow/internal/utilities"
	"net/http"
)

type AuthHandler struct{}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
}

func (handler *AuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "仅支持 POST 请求")
		return
	}

	if utilities.IsProductionMode() {
		handler.handleProductionAuth(w, r)
		return
	}

	handler.handleDebugAuth(w, r)
}

func (handler *AuthHandler) handleDebugAuth(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.UserID == "" {
		req.UserID = "anonymous"
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":      "debug-token-" + req.UserID,
		"expires_at": 0,
		"user_id":    req.UserID,
		"ttl":        "unlimited",
	})
}

// handleProductionAuth 在生产模式下执行真实的认证逻辑。
// 用户需要根据自己的业务需求实现以下逻辑：
//
// 1. 验证客户端提供的凭证（API Key、用户名密码、OAuth token 等）
// 2. 验证用户身份（查询数据库、调用认证服务等）
// 3. 生成并返回有效的认证 Token
//
// 当前为占位实现，请根据实际需求修改：
//   - 修改请求参数结构以匹配你的认证方式
//   - 添加数据库查询或外部认证服务调用
//   - 实现安全的 Token 生成逻辑（建议使用 JWT 或 HMAC 签名）
//   - 添加速率限制和防暴力破解措施
func (handler *AuthHandler) handleProductionAuth(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"api_key"`
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_body", "请求体格式错误")
		return
	}

	if req.APIKey == "" {
		req.APIKey = r.Header.Get("X-API-Key")
	}
	if req.APIKey == "" {
		req.APIKey = r.URL.Query().Get("api_key")
	}

	if req.APIKey == "" {
		writeJSONError(w, http.StatusUnauthorized, "missing_api_key", "缺少 API Key")
		return
	}

	if req.UserID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_user_id", "缺少用户 ID")
		return
	}

	// TODO: 用户需要实现的认证逻辑
	// ============================================
	// 1. 验证 API Key 是否有效（查询数据库或配置）
	//    validAPIKey := verifyAPIKey(req.APIKey)
	//    if !validAPIKey {
	//        writeJSONError(w, http.StatusUnauthorized, "invalid_api_key", "API Key 无效")
	//        return
	//    }
	//
	// 2. 验证用户身份（可选）
	//    user := getUserByID(req.UserID)
	//    if user == nil {
	//        writeJSONError(w, http.StatusNotFound, "user_not_found", "用户不存在")
	//        return
	//    }
	//
	// 3. 生成安全的 Token（建议使用 JWT 或 HMAC）
	//    token, expiresAt := generateSecureToken(req.UserID)
	// ============================================

	// 以下为示例代码，生产环境必须替换为真实实现
	_ = req.APIKey // 移除警告：生产环境需要使用此变量

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":      "production-token-" + req.UserID,
		"expires_at": 0,
		"user_id":    req.UserID,
		"ttl":        "24h",
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error":   code,
		"message": message,
	})
}

func RegisterAuthHandlers(mux *http.ServeMux, authHandler *AuthHandler) {
	mux.Handle("/api/auth/token", authHandler)
}
