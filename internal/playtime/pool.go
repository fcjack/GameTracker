package playtime

import (
	"context"
	"log/slog"
	"sync"
)

// WorkerPool consumes playtime events from a channel using a fixed worker count.
type WorkerPool struct {
	events  chan Event
	handler *Handler
	workers int

	wg     sync.WaitGroup
	closed bool
	mu     sync.Mutex
}

func NewWorkerPool(handler *Handler, workers, queueSize int) *WorkerPool {
	if workers < 1 {
		workers = 1
	}
	if queueSize < 1 {
		queueSize = 1
	}
	return &WorkerPool{
		events:  make(chan Event, queueSize),
		handler: handler,
		workers: workers,
	}
}

func (p *WorkerPool) Publish(event Event) {
	if p == nil {
		return
	}
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		slog.Warn("playtime event dropped, worker pool stopped",
			"kind", event.Kind,
			"user_id", event.UserID,
		)
		return
	}
	p.events <- event
}

func (p *WorkerPool) Run(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case event, ok := <-p.events:
					if !ok {
						return
					}
					if err := p.handler.Process(ctx, event); err != nil {
						slog.Warn("playtime event failed",
							"kind", event.Kind,
							"user_id", event.UserID,
							"title_id", event.TitleID,
							"app_id", event.AppID,
							"error", err,
						)
					}
				}
			}
		}()
	}
}

func (p *WorkerPool) Stop() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	close(p.events)
	p.mu.Unlock()
	p.wg.Wait()
}

// Pending returns the number of events waiting to be processed.
func (p *WorkerPool) Pending() int {
	return len(p.events)
}
