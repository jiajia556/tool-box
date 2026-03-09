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

	// Printf is optional logger (e.g. fmt.Printf). If nil, logging is disabled.
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

func (r *Runner) Run(ctx context.Context) error {
	if r.interval <= 0 {
		return errors.New("runner: interval must be > 0")
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
