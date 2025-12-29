package ctp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"hhwtrade.com/internal/model"
)

// Client handles all outgoing communication to the CTP Core via Redis.
type Client struct {
	rdb *redis.Client
}

// NewClient creates a new CTP Client.
func NewClient(rdb *redis.Client) *Client {
	return &Client{rdb: rdb}
}

// SendCommand pushes a unified command to the Redis list.
func (c *Client) SendCommand(ctx context.Context, cmd Command) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}
	if err := c.rdb.LPush(ctx, InCtpCmdQueue, data).Err(); err != nil {
		return fmt.Errorf("failed to push command to redis: %w", err)
	}
	return nil
}

// Subscribe sends a subscription request for a specific instrument.
func (c *Client) Subscribe(ctx context.Context, instrumentID string) error {
	cmd := Command{
		Type: "SUBSCRIBE",
		Payload: map[string]interface{}{
			"InstrumentID": instrumentID,
		},
		RequestID: fmt.Sprintf("sub-%s-%s", instrumentID, time.Now().Format("20060102150405")),
	}
	return c.SendCommand(ctx, cmd)
}

// Unsubscribe sends an unsubscribe request.
func (c *Client) Unsubscribe(ctx context.Context, instrumentID string) error {
	cmd := Command{
		Type: "UNSUBSCRIBE",
		Payload: map[string]interface{}{
			"InstrumentID": instrumentID,
		},
		RequestID: fmt.Sprintf("unsub-%s-%s", instrumentID, time.Now().Format("20060102150405")),
	}
	return c.SendCommand(ctx, cmd)
}

// QueryPositions requests all positions for a user and instrument.
func (c *Client) QueryPositions(ctx context.Context, userID string, instrumentID string) error {
	cmd := Command{
		Type: "QUERY_POSITIONS",
		Payload: map[string]interface{}{
			"InvestorID":   userID,
			"InstrumentID": instrumentID,
		},
		RequestID: fmt.Sprintf("query-pos-%s", time.Now().Format("20060102150405")),
	}
	return c.SendCommand(ctx, cmd)
}

// QueryAccount requests trading account info.
func (c *Client) QueryAccount(ctx context.Context, userID string) error {
	cmd := Command{
		Type: "QUERY_ACCOUNT",
		Payload: map[string]interface{}{
			"InvestorID": userID,
		},
		RequestID: fmt.Sprintf("query-acc-%s", time.Now().Format("20060102150405")),
	}
	return c.SendCommand(ctx, cmd)
}

// SyncInstruments triggers a global instrument sync.
func (c *Client) SyncInstruments(ctx context.Context) error {
	cmd := Command{
		Type:      "QUERY_INSTRUMENTS",
		Payload:   map[string]interface{}{},
		RequestID: fmt.Sprintf("sync-inst-%s", time.Now().Format("20060102150405")),
	}
	return c.SendCommand(ctx, cmd)
}

// InsertOrder sends an order insertion command.
// This encapsulates the params conversion logic previously found in strategies.
func (c *Client) InsertOrder(ctx context.Context, order *model.Order) error {
	// Construct the payload for CTP
	// Note: We are passing the raw characters '0','1' etc directly as they are stored in model
	payload := map[string]interface{}{
		"InstrumentID": order.InstrumentID,
		"ExchangeID":   order.ExchangeID,
		"OrderRef":     order.OrderRef,
		"Direction":    string(order.Direction),
		"OffsetFlag":   string(order.CombOffsetFlag),
		"Price":        order.LimitPrice,
		"Volume":       order.VolumeTotalOriginal,
		"OrderPriceType": "LimitPrice", // Defaulting to LimitPrice for now
		"TimeCondition": "GFD",        // Default
		"UserID":       order.UserID,
		"InvestorID":   order.InvestorID,
	// Add StrategyID to payload if needed by CTP? No, CTP doesn't know StrategyID, 
	// but we map it back via OrderRef in the database.
	}
	
	// If it's a generated order, ensure these IDs are set
	if order.InvestorID == "" {
		payload["InvestorID"] = order.UserID // Fallback
	}

	cmd := Command{
		Type:      "INSERT_ORDER",
		Payload:   payload,
		RequestID: order.OrderRef, // Use OrderRef as RequestID for traceability
	}
	return c.SendCommand(ctx, cmd)
}

// CancelOrder sends an order cancellation command.
func (c *Client) CancelOrder(ctx context.Context, order *model.Order) error {
	cmd := Command{
		Type: "CANCEL_ORDER",
		Payload: map[string]interface{}{
			"InstrumentID": order.InstrumentID,
			"OrderRef":     order.OrderRef,
			"ExchangeID":   order.ExchangeID,
			"FrontID":      order.FrontID,
			"SessionID":    order.SessionID,
			"ActionFlag":   "0", // '0' is Delete (撤单)
		},
		RequestID: "cancel-" + order.OrderRef,
	}
	return c.SendCommand(ctx, cmd)
}
