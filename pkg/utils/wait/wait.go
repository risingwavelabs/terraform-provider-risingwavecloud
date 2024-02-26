package wait

import (
	"context"
	"time"
)

type PollingParams struct {
	Interval time.Duration
	Timeout  time.Duration
}

func Poll(ctx context.Context, callback func() (bool, error), params PollingParams) error {
	ctx, cancel := context.WithTimeout(ctx, params.Timeout)
	defer cancel()

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
		}
	}
}
