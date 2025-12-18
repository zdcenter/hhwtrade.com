package infra

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const (
	CommandQueueKey  = "ctp_command_queue"  // Go -> CTP (RPUSH -> LPOP)
	ResponseQueueKey = "ctp_response_queue" // CTP -> Go (LPUSH -> RPOP)
)

// TradeResponse represents the message sent from CTP Core to Go.
type TradeResponse struct {
	Type      string      `json:"type"`       // "RTN_ORDER", "RTN_TRADE", "ERR_ORDER"
	RequestID string      `json:"request_id"` // Matches the UUID sent in TradeCommand
	Payload   interface{} `json:"payload"`    // Dynamic content (Order status, Trade details)
}

type TradeCommand struct {
	Type      string      `json:"type"` // e.g., "INSERT_ORDER", "CANCEL_ORDER", "SUBSCRIBE"
	Payload   interface{} `json:"payload"`
	RequestID string      `json:"request_id"`
}

// SendTradeCommand pushes a command to the Redis list for CTP Core to consume.
// Direction: Go -> CTP. Action: RPUSH (Append to Right). CTP should LPOP.
func SendTradeCommand(ctx context.Context, rdb *redis.Client, cmd TradeCommand) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	if err := rdb.RPush(ctx, CommandQueueKey, data).Err(); err != nil {
		return fmt.Errorf("failed to push command to redis: %w", err)
	}
	return nil
}

// PopCtpResponse retrieves a response from CTP Core.
// Direction: CTP -> Go. Action: RPOP (Take from Right). CTP should have LPUSHed.
func PopCtpResponse(ctx context.Context, rdb *redis.Client) (string, error) {
	// RPop returns the element or redis.Nil
	result, err := rdb.RPop(ctx, ResponseQueueKey).Result()
	if err == redis.Nil {
		return "", nil // Empty queue
	}
	if err != nil {
		return "", fmt.Errorf("failed to pop response from redis: %w", err)
	}
	return result, nil
}
