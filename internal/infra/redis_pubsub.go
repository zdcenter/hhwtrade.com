package infra

import (
	"context"
	"log"
	"strings"

	"github.com/redis/go-redis/v9"
)

// MarketMessage wraps the topic and the data payload.
type MarketMessage struct {
	Symbol  string `json:"symbol"`
	Payload string `json:"payload"`
}

// MarketDataChan is now a channel of MarketMessage.
var MarketDataChan = make(chan MarketMessage, 1000)

// StartMarketDataSubscriber starts a goroutine to subscribe to market data.
func StartMarketDataSubscriber(rdb *redis.Client, ctx context.Context) {
	// Subscribe to all channels matching pattern
	pubsub := rdb.PSubscribe(ctx, "market.*")

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
			// Strip "market." prefix to get the actual symbol
			symbol := strings.TrimPrefix(msg.Channel, "market.")

			// Forward payload to internal channel non-blocking
			message := MarketMessage{
				Symbol:  symbol,
				Payload: msg.Payload,
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
