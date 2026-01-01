package constants

// Redis 队列名称
const (
	// RedisQueueCTPCommand Go → CTP 的指令队列
	RedisQueueCTPCommand = "ctp_cmd_queue"

	// RedisQueueCTPResponse CTP → Go 的交易回报队列
	RedisQueueCTPResponse = "ctp_response_queue"
)

// Redis Pub/Sub 频道
const (
	// RedisPubSubMarketPrefix 行情数据频道前缀
	RedisPubSubMarketPrefix = "market."

	// RedisPubSubQuery 查询结果频道
	RedisPubSubQuery = "ctp_query_returns"

	// RedisPubSubStatus CTP 核心状态频道 (用于重启恢复)
	RedisPubSubStatus = "ctp.status"

	// StatusConnected CTP 已连接状态消息
	StatusConnected = "connected"
)
