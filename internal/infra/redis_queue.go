package infra

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const (
	// [Go -> CTP] 指令队列 (List)
	InCtpCmdQueue = "ctp_cmd_queue"

	// [CTP -> Go] 交易/成交回报队列 (List)
	PushCtpTradeReportList = "ctp_response_queue"

	// [CTP -> Go] 主动查询结果频道 (Pub/Sub)
	PubCtpQueryReplyChan = "ctp_query_returns"

	// [CTP -> Go] 行情数据频道前缀 (Pub/Sub)
	PubCtpMarketDataPrefix = "market."
)

// TradeResponse represents the message sent from CTP Core to Go.
type TradeResponse struct {
	Type      string      `json:"type"`       // "RTN_ORDER", "RTN_TRADE", "ERR_ORDER"
	Payload   interface{} `json:"payload"`    // Dynamic content (Order status, Trade details)
	RequestID string      `json:"request_id"` // Matches the UUID sent in TradeCommand
}

// Command represents a unified instruction sent from Go to CTP Core.
type Command struct {
	Type      string                 `json:"type"`       // Big uppercase, e.g., "SUBSCRIBE", "INSERT_ORDER"
	RequestID string                 `json:"request_id"` // Optional/Query mandatory
	Payload   map[string]interface{} `json:"payload"`    // All parameters here
}

// SendCommand pushes a unified command to the Redis list for CTP Core to consume.
func SendCommand(ctx context.Context, rdb *redis.Client, cmd Command) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	// Use LPUSH to match user requirements
	if err := rdb.LPush(ctx, InCtpCmdQueue, data).Err(); err != nil {
		return fmt.Errorf("failed to push command to redis: %w", err)
	}
	return nil
}

// PopCtpResponse retrieves a response from CTP Core.
// Direction: CTP -> Go. Action: RPOP (Take from Right). CTP should have LPUSHed.
func PopCtpResponse(ctx context.Context, rdb *redis.Client) (string, error) {
	// RPop returns the element or redis.Nil
	result, err := rdb.RPop(ctx, PushCtpTradeReportList).Result()
	if err == redis.Nil {
		return "", nil // Empty queue
	}
	if err != nil {
		return "", fmt.Errorf("failed to pop response from redis: %w", err)
	}
	return result, nil
}
