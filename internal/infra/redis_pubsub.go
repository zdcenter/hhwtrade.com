package infra

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/redis/go-redis/v9"
)

// MarketMessage is used for internal routing between Redis and WebSocket/Engine.
type MarketMessage struct {
	Symbol  string          `json:"-"`       // Internal routing key (e.g. "rb2605")
	Payload json.RawMessage `json:"payload"` // Raw CTP JSON data
}

// MarketDataChan is now a channel of MarketMessage.
var MarketDataChan = make(chan MarketMessage, 10000)

// StartMarketDataSubscriber starts a goroutine to subscribe to market data.
func StartMarketDataSubscriber(rdb *redis.Client, ctx context.Context) {
	// Subscribe to all channels matching pattern
	pattern := PubCtpMarketDataPrefix + "*"
	pubsub := rdb.PSubscribe(ctx, pattern)

	// Wait for confirmation that subscription is created
	_, err := pubsub.Receive(ctx)
	if err != nil {
		log.Fatalf("Failed to subscribe to market data: %v", err)
	}

	ch := pubsub.Channel()

	go func() {
		defer pubsub.Close()
		log.Println("Started Market Data Subscriber Loop")
		for msg := range ch {
			// Skip empty payloads
			payload := strings.TrimSpace(msg.Payload)
			if payload == "" {
				continue
			}

			// Defensive: Validate JSON before wrapping in RawMessage
			// If CTP core sends truncated JSON, this will catch it
			if !json.Valid([]byte(payload)) {
				log.Printf("Warning: Dropping invalid JSON from Redis channel %s: %s", msg.Channel, payload)
				continue
			}

			// Strip prefix to get the actual symbol
			symbol := strings.TrimPrefix(msg.Channel, PubCtpMarketDataPrefix)

			// Forward payload to internal channel non-blocking
			message := MarketMessage{
				Symbol:  symbol,
				Payload: json.RawMessage(payload),
			}

			select {
			case MarketDataChan <- message:
				// Data sent
			default:
				log.Println("Warning: MarketDataChan is full, dropping message")
			}
		}
	}()
}

// StartQueryReplySubscriber starts a goroutine to listen for query responses from CTP.
func StartQueryReplySubscriber(rdb *redis.Client, ctx context.Context) {
	pubsub := rdb.Subscribe(ctx, PubCtpQueryReplyChan)

	ch := pubsub.Channel()

	go func() {
		defer pubsub.Close()
		log.Println("Started Query Reply Subscriber Loop")
		for msg := range ch {
			payload := strings.TrimSpace(msg.Payload)
			if payload == "" {
				continue
			}

			// Defensive: Validate JSON from Query Reply channel
			if !json.Valid([]byte(payload)) {
				log.Printf("Warning: Dropping invalid JSON from Query Reply channel: %s", payload)
				continue
			}

			// Manual query responses don't have a symbol context in the channel name,
			// but they follow the same MarketMessage structure for engine processing.
			message := MarketMessage{
				Symbol:  "", // Not used for query routing to WS subscribers
				Payload: json.RawMessage(payload),
			}

			select {
			case MarketDataChan <- message:
			default:
				log.Println("Warning: MarketDataChan is full, dropping query reply")
			}
		}
	}()
}
