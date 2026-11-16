package saga_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/application/saga"
	"github.com/stretchr/testify/assert"
)

type expirerSpy struct {
	calls atomic.Int32
	err   error
}

func (e *expirerSpy) ExpireOverdue(context.Context) (int, error) {
	e.calls.Add(1)
	return 1, e.err
}

func runMonitorBriefly(expirer *expirerSpy) {
	monitorCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		saga.NewTimeoutMonitor(expirer).WithInterval(5 * time.Millisecond).Run(monitorCtx)
		close(done)
	}()
	time.Sleep(40 * time.Millisecond)
	cancel()
	<-done
}

func TestTimeoutMonitor_ShouldCheckOverdueSagasPeriodicallyUntilCancelled(t *testing.T) {
	expirer := &expirerSpy{}

	runMonitorBriefly(expirer)

	assert.GreaterOrEqual(t, expirer.calls.Load(), int32(2))
}

func TestTimeoutMonitor_ShouldKeepRunningWhenCheckFails(t *testing.T) {
	expirer := &expirerSpy{err: errDatabase}

	runMonitorBriefly(expirer)

	assert.GreaterOrEqual(t, expirer.calls.Load(), int32(2))
}
