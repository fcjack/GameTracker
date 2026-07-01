package playtime

import "testing"

func TestWorkerPoolQueueCapacity(t *testing.T) {
	t.Parallel()

	pool := NewWorkerPool(NewHandler(nil, nil, nil), 1, 2)
	pool.Publish(Event{Kind: KindSteam, UserID: 1, AppID: 570, Minutes: 5})
	pool.Publish(Event{Kind: KindSteam, UserID: 1, AppID: 730, Minutes: 10})

	if got := pool.Pending(); got != 2 {
		t.Fatalf("Pending() = %d, want 2", got)
	}
}
