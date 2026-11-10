package web

import (
	"context"
	"sync"
	"time"
)

type pollingControl struct {
	mu       sync.RWMutex
	interval time.Duration
	changed  chan struct{}
}

func newPollingControl(seconds int) *pollingControl {
	return &pollingControl{interval: time.Duration(seconds) * time.Second, changed: make(chan struct{})}
}

func (m *pollingControl) pollSnapshot() (time.Duration, <-chan struct{}) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.interval, m.changed
}

func (m *pollingControl) setPollInterval(interval time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.interval == interval {
		return
	}
	m.interval = interval
	close(m.changed)
	m.changed = make(chan struct{})
}

// 配置修改后，按同一次查询结束时间重新计算等待，不取消签到倒计时。
func (m *pollingControl) waitForPoll(ctx context.Context, lastQuery time.Time) bool {
	for {
		if ctx.Err() != nil {
			return false
		}
		interval, changed := m.pollSnapshot()
		timer := time.NewTimer(time.Until(lastQuery.Add(interval)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-changed:
			timer.Stop()
			continue
		case <-timer.C:
			// 若修改设置与计时器同时触发，先应用新间隔。
			select {
			case <-changed:
				continue
			default:
			}
			return ctx.Err() == nil
		}
	}
}
