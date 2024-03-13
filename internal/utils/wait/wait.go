package wait

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

var (
	ErrWaitTimeout = errors.New("timeout while waiting")
)

type PollingParams struct {
	Interval time.Duration
	Timeout  time.Duration
}

func Poll(ctx context.Context, callback func() (bool, error), params PollingParams) error {
	timer := time.NewTimer(params.Timeout)
	defer timer.Stop()

	ticker := time.NewTicker(params.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			ok, err := callback()
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
		case <-timer.C:
			return ErrWaitTimeout
		}
	}
}
