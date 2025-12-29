package ctp

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
	Type      string      `json:"Type"`       // "RTN_ORDER", "RTN_TRADE", "ERR_ORDER"
	Payload   interface{} `json:"Payload"`    // Dynamic content (Order status, Trade details)
	RequestID string      `json:"RequestID"` // Matches the UUID sent in TradeCommand
}

// Command represents a unified instruction sent from Go to CTP Core.
type Command struct {
	Type      string                 `json:"Type"`       // Big uppercase, e.g., "SUBSCRIBE", "INSERT_ORDER"
	RequestID string                 `json:"RequestID"` // Optional/Query mandatory
	Payload   map[string]interface{} `json:"Payload"`    // All parameters here
}
