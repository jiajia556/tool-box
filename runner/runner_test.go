package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func captureLogger() (func(string, ...any) (int, error), *logSink) {
	sink := &logSink{}
	return sink.printf, sink
}

type logSink struct {
	mu   sync.Mutex
	logs []string
}

func (s *logSink) printf(format string, args ...any) (int, error) {
	msg := fmt.Sprintf(format, args...)
	msg = strings.TrimSuffix(msg, "\n")
	msg = strings.TrimSuffix(msg, "\r")

	s.mu.Lock()
	s.logs = append(s.logs, msg)
	s.mu.Unlock()
	return len(msg), nil
}

func (s *logSink) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.logs))
	copy(out, s.logs)
	return out
}

func TestRun_InvalidInterval(t *testing.T) {
	r := New(0)
	r.Printf = func(string, ...any) (int, error) { return 0, nil }
	if err := r.Run(context.Background()); err == nil {
		t.Fatalf("expected error for invalid interval")
	}
}

func TestRun_TaskExecutedAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var count int32
	r := New(10*time.Millisecond, func(context.Context) {
		atomic.AddInt32(&count, 1)
		cancel()
	})
	r.Printf = func(string, ...any) (int, error) { return 0, nil }

	if err := r.Run(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&count); got != 1 {
		t.Fatalf("expected 1 task execution, got %d", got)
	}
}

func TestRun_ContextCanceledBeforeRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var count int32
	r := New(10*time.Millisecond, func(context.Context) {
		atomic.AddInt32(&count, 1)
	})
	r.Printf = func(string, ...any) (int, error) { return 0, nil }

	if err := r.Run(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&count); got != 0 {
		t.Fatalf("expected 0 task executions, got %d", got)
	}
}

func TestRun_LoggingOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	printf, sink := captureLogger()
	r := New(10*time.Millisecond, func(context.Context) { cancel() })
	r.Printf = printf

	if err := r.Run(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logs := sink.snapshot()
	want := []string{"程序已启动...", "start", "ok", "正在退出，请稍后...", "退出成功！"}
	pos := 0
	for _, line := range logs {
		if pos < len(want) && line == want[pos] {
			pos++
		}
	}
	if pos != len(want) {
		t.Fatalf("expected log sequence %v, got %v", want, logs)
	}
}

func TestRun_WithTrackedWaitBlocksUntilDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := New(10*time.Millisecond, func(context.Context) { cancel() })
	WithTrackedWait()(r)
	r.Printf = func(string, ...any) (int, error) { return 0, nil }
	r.TrackAdd(1)

	done := make(chan struct{})
	go func() {
		_ = r.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		t.Fatalf("runner returned before TrackDone")
	case <-time.After(50 * time.Millisecond):
	}

	r.TrackDone()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("runner did not return after TrackDone")
	}
}

func TestRun_TaskCanUseTrackerFromContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	release := make(chan struct{})
	returned := make(chan struct{})
	errCh := make(chan error, 1)

	r := New(10*time.Millisecond, func(ctx context.Context) {
		if ok := TrackAdd(ctx, 1); !ok {
			errCh <- errors.New("tracker not available in task context")
			return
		}
		go func() {
			<-release
			_ = TrackDone(ctx)
		}()
		cancel()
	})
	WithTrackedWait()(r)
	r.Printf = func(string, ...any) (int, error) { return 0, nil }

	go func() {
		_ = r.Run(ctx)
		close(returned)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("task error: %v", err)
	case <-returned:
		t.Fatalf("runner returned before TrackDone")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	select {
	case <-returned:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("runner did not return after TrackDone")
	}
}

func TestTrackerHelpers_NoTracker(t *testing.T) {
	ctx := context.Background()
	if ok := TrackAdd(ctx, 1); ok {
		t.Fatalf("expected TrackAdd to return false without tracker")
	}
	if ok := TrackDone(ctx); ok {
		t.Fatalf("expected TrackDone to return false without tracker")
	}
	if ok := SafeTrackAdd(ctx, 1); ok {
		t.Fatalf("expected SafeTrackAdd to return false without tracker")
	}
	if ok := SafeTrackDone(ctx); ok {
		t.Fatalf("expected SafeTrackDone to return false without tracker")
	}
}

func TestSafeTrackDone_Recovers(t *testing.T) {
	r := New(10 * time.Millisecond)
	ctx := WithTracker(context.Background(), r)

	if ok := SafeTrackDone(ctx); ok {
		t.Fatalf("expected SafeTrackDone to return false when counter would go negative")
	}
}
