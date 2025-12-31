package event

import (
	"context"
	"log"
	"sync"
	"time"
)

// Event 表示系统中的一个事件
type Event struct {
	Type      string                 // 事件类型
	Source    string                 // 事件来源
	Data      interface{}            // 事件数据
	Metadata  map[string]interface{} // 元数据
	Timestamp time.Time              // 时间戳
}

// Handler 事件处理函数
type Handler func(ctx context.Context, event Event) error

// Bus 事件总线，用于解耦系统各个组件
type Bus struct {
	handlers map[string][]Handler
	mu       sync.RWMutex

	// 异步处理的缓冲通道
	eventChan chan Event
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// NewBus 创建新的事件总线
func NewBus(bufferSize int) *Bus {
	ctx, cancel := context.WithCancel(context.Background())

	bus := &Bus{
		handlers:  make(map[string][]Handler),
		eventChan: make(chan Event, bufferSize),
		ctx:       ctx,
		cancel:    cancel,
	}

	// 启动事件处理协程
	bus.wg.Add(1)
	go bus.processEvents()

	return bus
}

// Subscribe 订阅事件类型
func (b *Bus) Subscribe(eventType string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.handlers[eventType] == nil {
		b.handlers[eventType] = make([]Handler, 0)
	}
	b.handlers[eventType] = append(b.handlers[eventType], handler)

	log.Printf("EventBus: Subscribed to event type: %s", eventType)
}

// Publish 发布事件（异步）
func (b *Bus) Publish(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	select {
	case b.eventChan <- event:
		// 成功发送
	default:
		log.Printf("EventBus: Warning - event channel full, dropping event: %s", event.Type)
	}
}

// PublishSync 同步发布事件（立即处理）
func (b *Bus) PublishSync(ctx context.Context, event Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	return b.dispatch(ctx, event)
}

// processEvents 处理事件的后台协程
func (b *Bus) processEvents() {
	defer b.wg.Done()

	for {
		select {
		case event := <-b.eventChan:
			if err := b.dispatch(b.ctx, event); err != nil {
				log.Printf("EventBus: Error processing event %s: %v", event.Type, err)
			}
		case <-b.ctx.Done():
			log.Println("EventBus: Shutting down event processor")
			return
		}
	}
}

// dispatch 分发事件给所有订阅者
func (b *Bus) dispatch(ctx context.Context, event Event) error {
	b.mu.RLock()
	handlers := b.handlers[event.Type]
	b.mu.RUnlock()

	if len(handlers) == 0 {
		// 没有订阅者，这是正常的
		return nil
	}

	// 并发执行所有处理器
	var wg sync.WaitGroup
	errChan := make(chan error, len(handlers))

	for _, handler := range handlers {
		wg.Add(1)
		go func(h Handler) {
			defer wg.Done()
			if err := h(ctx, event); err != nil {
				errChan <- err
			}
		}(handler)
	}

	wg.Wait()
	close(errChan)

	// 收集错误（如果有）
	for err := range errChan {
		if err != nil {
			log.Printf("EventBus: Handler error for event %s: %v", event.Type, err)
		}
	}

	return nil
}

// Shutdown 关闭事件总线
func (b *Bus) Shutdown() {
	log.Println("EventBus: Shutting down...")
	b.cancel()
	b.wg.Wait()
	close(b.eventChan)
	log.Println("EventBus: Shutdown complete")
}

// GetSubscriberCount 获取某个事件类型的订阅者数量（用于调试）
func (b *Bus) GetSubscriberCount(eventType string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.handlers[eventType])
}
