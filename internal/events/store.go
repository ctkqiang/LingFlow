package events

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// EventStore 定义事件追加与读取能力。
type EventStore interface {
	Append(ctx context.Context, event DomainEvent) (DomainEvent, error)
	LoadStream(ctx context.Context, streamID string) ([]DomainEvent, error)
	LoadAll(ctx context.Context) ([]DomainEvent, error)
}

// InMemoryEventStore 是进程内事件存储实现。
//
// 当前项目没有数据库依赖，因此先使用内存存储承载 event sourcing 结构。
// 之后如果需要持久化，可以在不改 handler 逻辑的情况下替换 EventStore 实现。
type InMemoryEventStore struct {
	storeMutex sync.RWMutex
	streams    map[string][]DomainEvent
	events     []DomainEvent
}

// NewInMemoryEventStore 创建一个进程内事件存储。
func NewInMemoryEventStore() *InMemoryEventStore {
	return &InMemoryEventStore{
		streams: make(map[string][]DomainEvent),
		events:  make([]DomainEvent, 0),
	}
}

// Append 将事件追加到指定 stream，并分配该 stream 内的递增版本号。
func (store *InMemoryEventStore) Append(
	ctx context.Context,
	event DomainEvent,
) (DomainEvent, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return DomainEvent{}, ctxErr
	}

	if event.StreamID == "" {
		return DomainEvent{}, fmt.Errorf("事件 StreamID 不能为空")
	}
	if event.EventType == "" {
		return DomainEvent{}, fmt.Errorf("事件 EventType 不能为空")
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	if event.EventID == "" {
		event.EventID = newEventIdentifier()
	}

	store.storeMutex.Lock()
	defer store.storeMutex.Unlock()

	event.Version = int64(len(store.streams[event.StreamID]) + 1)
	store.streams[event.StreamID] = append(store.streams[event.StreamID], event)
	store.events = append(store.events, event)

	return event, nil
}

// LoadStream 返回指定 stream 的事件副本。
func (store *InMemoryEventStore) LoadStream(
	ctx context.Context,
	streamID string,
) ([]DomainEvent, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	store.storeMutex.RLock()
	defer store.storeMutex.RUnlock()

	streamEvents := store.streams[streamID]
	result := make([]DomainEvent, len(streamEvents))
	copy(result, streamEvents)
	return result, nil
}

// LoadAll 返回当前进程内的全部事件副本。
func (store *InMemoryEventStore) LoadAll(ctx context.Context) ([]DomainEvent, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	store.storeMutex.RLock()
	defer store.storeMutex.RUnlock()

	result := make([]DomainEvent, len(store.events))
	copy(result, store.events)
	return result, nil
}
