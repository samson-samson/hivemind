// Package eventbus 内存事件总线：P0 单实例下用于 SSE 实时推送。
// 接口层出 SSE 供前端实时（见 api/sse.go）。
package eventbus

import (
	"sync"
	"sync/atomic"
	"time"
)

// Event 一条领域事件。
type Event struct {
	Type       string    `json:"type"`        // 事件类型，如 incident.created
	IncidentID string    `json:"incident_id"` // 作用域：归属事故
	Timestamp  time.Time `json:"timestamp"`
	Data       any       `json:"data,omitempty"`
}

// Subscription 一个订阅通道。
type Subscription struct {
	C  <-chan Event
	id uint64
}

// Bus 按事故作用域分发事件的发布/订阅总线。
// 消费方需及时消费；通道缓冲满时丢弃最早事件（SSE 慢消费者保护，避免阻塞主流程）。
// 注意：丢旧保新（监控语义——新状态比旧状态重要），并统计丢弃数供告警。
type Bus struct {
	mu      sync.RWMutex
	nextID  uint64
	dropped int64
	// subs[incidentID] -> map[subID]chan
	subs map[string]map[uint64]chan Event
}

const bufSize = 64

// NewBus 构造事件总线。
func NewBus() *Bus {
	return &Bus{subs: make(map[string]map[uint64]chan Event)}
}

// Publish 向某事故的所有订阅者广播事件。
func (b *Bus) Publish(incidentID, typ string, data any) {
	ev := Event{
		Type:       typ,
		IncidentID: incidentID,
		Timestamp:  time.Now().UTC(),
		Data:       data,
	}
	b.mu.RLock()
	chans := b.subs[incidentID]
	for _, ch := range chans {
		select {
		case ch <- ev:
		default:
			// 慢消费者：先丢弃通道里最早的事件，再尝试放入新事件。
			// （此前实现丢的是"当前最新"，与注释相反——已修复为丢旧保新。）
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- ev:
			default:
				atomic.AddInt64(&b.dropped, 1)
			}
		}
	}
	b.mu.RUnlock()
}

// DroppedCount 累计丢弃事件数（SSE 慢消费者告警用）。
func (b *Bus) DroppedCount() int64 { return atomic.LoadInt64(&b.dropped) }

// Subscribe 订阅某事故的事件流。
func (b *Bus) Subscribe(incidentID string) *Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.subs[incidentID]; !ok {
		b.subs[incidentID] = make(map[uint64]chan Event)
	}
	b.nextID++
	ch := make(chan Event, bufSize)
	b.subs[incidentID][b.nextID] = ch
	return &Subscription{C: ch, id: b.nextID}
}

// Unsubscribe 取消订阅。
func (b *Bus) Unsubscribe(incidentID string, sub *Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if chans, ok := b.subs[incidentID]; ok {
		if ch, ok := chans[sub.id]; ok {
			delete(chans, sub.id)
			close(ch)
		}
	}
}
