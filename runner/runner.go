package runner

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Runner struct {
	interval time.Duration
	tasks    []func(context.Context)

	inFlight sync.WaitGroup
	tracked  sync.WaitGroup

	waitTracked bool

	// Printf 是可选日志函数（例如 fmt.Printf），为 nil 时不输出日志。
	Printf func(format string, args ...any) (int, error)
}

type Option func(*Runner)

func WithLogger(printf func(format string, args ...any) (int, error)) Option {
	return func(r *Runner) { r.Printf = printf }
}

func WithoutLogger() Option {
	return func(r *Runner) { r.Printf = nil }
}

func WithTrackedWait() Option {
	return func(r *Runner) { r.waitTracked = true }
}

func New(interval time.Duration, tasks ...func(context.Context)) *Runner {
	return &Runner{
		interval: interval,
		tasks:    append([]func(context.Context){}, tasks...),
		Printf:   fmt.Printf,
	}
}

func (r *Runner) Add(task func(context.Context)) {
	r.tasks = append(r.tasks, task)
}

func (r *Runner) TrackAdd(delta int) { r.tracked.Add(delta) }
func (r *Runner) TrackDone()         { r.tracked.Done() }

// Tracker 用于任务上报异步工作量，便于退出时等待收尾。
type Tracker interface {
	TrackAdd(delta int)
	TrackDone()
}

type trackerKey struct{}

// WithTracker 将 Tracker 写入 context，供任务内使用。
func WithTracker(ctx context.Context, t Tracker) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, trackerKey{}, t)
}

// TrackerFromContext 获取 context 中的 Tracker。
func TrackerFromContext(ctx context.Context) (Tracker, bool) {
	if ctx == nil {
		return nil, false
	}
	v := ctx.Value(trackerKey{})
	if v == nil {
		return nil, false
	}
	t, ok := v.(Tracker)
	return t, ok
}

// TrackAdd 在 context 中存在 Tracker 时上报新增工作量。
func TrackAdd(ctx context.Context, delta int) bool {
	t, ok := TrackerFromContext(ctx)
	if !ok {
		return false
	}
	t.TrackAdd(delta)
	return true
}

// TrackDone 在 context 中存在 Tracker 时标记完成。
func TrackDone(ctx context.Context) bool {
	t, ok := TrackerFromContext(ctx)
	if !ok {
		return false
	}
	t.TrackDone()
	return true
}

// SafeTrackAdd 等同于 TrackAdd，但会捕获计数器的 panic。
func SafeTrackAdd(ctx context.Context, delta int) (ok bool) {
	t, ok := TrackerFromContext(ctx)
	if !ok {
		return false
	}
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	t.TrackAdd(delta)
	return true
}

// SafeTrackDone 等同于 TrackDone，但会捕获计数器的 panic。
func SafeTrackDone(ctx context.Context) (ok bool) {
	t, ok := TrackerFromContext(ctx)
	if !ok {
		return false
	}
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	t.TrackDone()
	return true
}

func (r *Runner) Run(ctx context.Context) error {
	if r.interval <= 0 {
		return errors.New("runner: interval must be > 0")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, stop := signal.NotifyContext(ctx,
		syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT,
	)
	defer stop()

	r.logf("程序已启动...")

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		if err := r.runOnce(ctx); err != nil {
			break
		}

		select {
		case <-ctx.Done():
			break
		case <-ticker.C:
			continue
		}
		break
	}

	r.logf("正在退出，请稍后...")

	r.inFlight.Wait()
	if r.waitTracked {
		r.tracked.Wait()
	}

	r.logf("退出成功！")
	return nil
}

func (r *Runner) runOnce(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	ctx = WithTracker(ctx, r)

	r.logf("start")

	var batch sync.WaitGroup
	for _, task := range r.tasks {
		task := task
		batch.Add(1)

		r.inFlight.Add(1)
		go func() {
			defer batch.Done()
			defer r.inFlight.Done()
			task(ctx)
		}()
	}
	batch.Wait()

	r.logf("ok")
	return nil
}

func (r *Runner) logf(format string, args ...any) {
	if r.Printf != nil {
		_, _ = r.Printf(format+"\n", args...)
	}
}
