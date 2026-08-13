package promptmetrics

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alitto/pond/v2"
)

// AsyncPublisher 用有界 worker pool 异步写入采集事件.
// 队列满或写库失败都不影响主请求链路, 仅按采样节奏记录 warn.
type AsyncPublisher struct {
	repo         *Repository
	pool         pond.Pool
	writeTimeout time.Duration
	dropped      atomic.Uint64
	stopOnce     sync.Once
	stopDone     chan struct{}
}

// NewAsyncPublisher 创建异步发布器.
func NewAsyncPublisher(repo *Repository, workerCount, queueSize int, writeTimeout time.Duration) *AsyncPublisher {
	if workerCount <= 0 {
		workerCount = 4
	}
	if queueSize <= 0 {
		queueSize = 1024
	}
	if writeTimeout <= 0 {
		writeTimeout = 3 * time.Second
	}
	return &AsyncPublisher{
		repo:         repo,
		pool:         pond.NewPool(workerCount, pond.WithQueueSize(queueSize)),
		writeTimeout: writeTimeout,
		stopDone:     make(chan struct{}),
	}
}

// Publish 提交事件写入任务. 提交失败时直接丢弃, 保护网关主链路.
func (p *AsyncPublisher) Publish(event Event) {
	if p == nil || p.repo == nil || p.pool == nil || p.pool.Stopped() {
		return
	}
	if _, ok := p.pool.TrySubmit(func() {
		p.write(event)
	}); !ok {
		p.logDrop("queue_full")
	}
}

// Stop 停止 worker pool 并等待已入队任务完成.
func (p *AsyncPublisher) Stop(timeout time.Duration) {
	if p == nil || p.pool == nil {
		return
	}
	p.stopOnce.Do(func() {
		go func() {
			p.pool.StopAndWait()
			close(p.stopDone)
		}()
	})
	if timeout <= 0 {
		<-p.stopDone
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-p.stopDone:
	case <-timer.C:
		slog.Warn("Prompt metrics publisher stop timed out")
	}
}

func (p *AsyncPublisher) write(event Event) {
	ctx, cancel := context.WithTimeout(context.Background(), p.writeTimeout)
	defer cancel()
	if err := p.repo.Insert(ctx, event); err != nil {
		slog.Warn("Prompt metrics insert failed", "error", err)
	}
}

func (p *AsyncPublisher) logDrop(reason string) {
	n := p.dropped.Add(1)
	if n%256 != 1 {
		return
	}
	slog.Warn("Prompt metrics event dropped", "reason", reason, "dropped", n)
}
