package events

import (
	"context"
	"fmt"
	"sync"
)

// EventHandler 是事件处理函数的类型。
type EventHandler func(ctx context.Context, event DomainEvent) error

// EventBus 定义事件订阅与发布的抽象能力。
type EventBus interface {
	Subscribe(eventType EventType, handler EventHandler)
	Publish(ctx context.Context, events ...DomainEvent) error
}

// InMemoryEventBus 是进程内事件总线实现。
//
// 通过 EventType 将事件路由到对应的处理器列表，支持并发安全的订阅与发布。
// 发布时先以读锁获取处理器快照，再释放锁后逐个调用处理器，
// 避免处理器内部再次调用 Publish 时产生死锁。
type InMemoryEventBus struct {
	busMutex sync.RWMutex
	handlers map[EventType][]EventHandler
}

// NewInMemoryEventBus 创建一个进程内事件总线。
func NewInMemoryEventBus() *InMemoryEventBus {
	return &InMemoryEventBus{
		handlers: make(map[EventType][]EventHandler),
	}
}

// Subscribe 将处理器注册到指定事件类型。
func (bus *InMemoryEventBus) Subscribe(eventType EventType, handler EventHandler) {
	bus.busMutex.Lock()
	defer bus.busMutex.Unlock()
	bus.handlers[eventType] = append(bus.handlers[eventType], handler)
}

// Publish 将事件发布到所有已订阅的处理器。
//
// 对每个事件，先以读锁获取对应事件类型的处理器快照，
// 释放锁后再逐个调用处理器。如果任一处理器返回错误，
// 则立即返回该错误；所有处理器均成功时返回 nil。
func (bus *InMemoryEventBus) Publish(ctx context.Context, events ...DomainEvent) error {
	for _, event := range events {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		bus.busMutex.RLock()
		handlers := bus.handlers[event.EventType]
		snapshot := make([]EventHandler, len(handlers))
		copy(snapshot, handlers)
		bus.busMutex.RUnlock()

		for _, handler := range snapshot {
			if err := handler(ctx, event); err != nil {
				return fmt.Errorf("事件处理器执行失败 [event_type=%s]: %w", event.EventType, err)
			}
		}
	}
	return nil
}
