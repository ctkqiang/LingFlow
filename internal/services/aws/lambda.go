package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"ling_flow/internal/utilities"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// HandleLambdaRequest 启动 AWS Lambda WebSocket 入口。
//
// 该函数仅在 WebSocketRuntimeLambda 模式下调用，
// 适用于 API Gateway WebSocket API 的 $connect / $disconnect / $default 路由。
func HandleLambdaRequest() {
	lambda.Start(WebSocketHandler)
}

// WebSocketHandler 处理 API Gateway WebSocket 事件。
func WebSocketHandler(
	runtimeContext context.Context,
	websocketRequest events.APIGatewayWebsocketProxyRequest,
) (events.APIGatewayProxyResponse, error) {
	start := time.Now()
	routeKey := websocketRequest.RequestContext.RouteKey
	connectionIdentifier := websocketRequest.RequestContext.ConnectionID

	utilities.LogProgress(
		"LambdaWebSocket",
		"HandleRequest",
		"收到 API Gateway WebSocket 事件",
		fmt.Sprintf("route=%s", routeKey),
		fmt.Sprintf("connectionId=%s", connectionIdentifier),
	)

	switch routeKey {
	case "$connect":
		return lambdaJSONResponse(200, map[string]string{"status": "connected"})
	case "$disconnect":
		return lambdaJSONResponse(200, map[string]string{"status": "disconnected"})
	case "$default":
		return handleDefaultWebSocketMessage(runtimeContext, websocketRequest, start)
	default:
		return handleDefaultWebSocketMessage(runtimeContext, websocketRequest, start)
	}
}

func handleDefaultWebSocketMessage(
	_ context.Context,
	websocketRequest events.APIGatewayWebsocketProxyRequest,
	start time.Time,
) (events.APIGatewayProxyResponse, error) {
	var messagePayload map[string]interface{}
	if websocketRequest.Body != "" {
		if unmarshalError := json.Unmarshal([]byte(websocketRequest.Body), &messagePayload); unmarshalError != nil {
			utilities.LogWarn(
				"LambdaWebSocket",
				"DefaultMessage",
				fmt.Sprintf("消息不是 JSON，按原始文本处理: %v", unmarshalError),
				time.Since(start),
			)
			return lambdaResponse(200, websocketRequest.Body), nil
		}
	}

	responseBody, marshalError := json.Marshal(map[string]interface{}{
		"connectionId": websocketRequest.RequestContext.ConnectionID,
		"message":      messagePayload,
	})
	if marshalError != nil {
		return lambdaResponse(500, "failed to encode response"), marshalError
	}

	utilities.LogSuccess(
		"LambdaWebSocket",
		"DefaultMessage",
		time.Since(start),
		fmt.Sprintf("connectionId=%s", websocketRequest.RequestContext.ConnectionID),
	)

	return lambdaResponse(200, string(responseBody)), nil
}

func lambdaJSONResponse(
	statusCode int,
	responsePayload interface{},
) (events.APIGatewayProxyResponse, error) {
	responseBody, marshalError := json.Marshal(responsePayload)
	if marshalError != nil {
		return lambdaResponse(500, "failed to encode response"), marshalError
	}
	return lambdaResponse(statusCode, string(responseBody)), nil
}

func lambdaResponse(statusCode int, responseBody string) events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Body:       responseBody,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}
}
