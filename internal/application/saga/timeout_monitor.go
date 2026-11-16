package saga

import (
	"context"
	"log/slog"
	"time"
)

const defaultMonitorInterval = 30 * time.Second

type OverdueExpirer interface {
	ExpireOverdue(ctx context.Context) (int, error)
}

type TimeoutMonitor struct {
	expirer  OverdueExpirer
	interval time.Duration
}

func NewTimeoutMonitor(expirer OverdueExpirer) *TimeoutMonitor {
	return &TimeoutMonitor{expirer: expirer, interval: defaultMonitorInterval}
}

func (m *TimeoutMonitor) WithInterval(interval time.Duration) *TimeoutMonitor {
	m.interval = interval
	return m
}

func (m *TimeoutMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.check(ctx)
		}
	}
}

func (m *TimeoutMonitor) check(ctx context.Context) {
	expired, err := m.expirer.ExpireOverdue(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "saga.timeout_check_failed",
			slog.String("event", "saga.timeout_check_failed"),
			slog.String("error", err.Error()),
		)
		return
	}
	if expired > 0 {
		slog.InfoContext(ctx, "saga.timed_out",
			slog.String("event", "saga.timed_out"),
			slog.Int("sagas", expired),
		)
	}
}
