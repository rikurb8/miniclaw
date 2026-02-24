package agent

import (
	"context"
	"errors"
	"time"
)

func (i *Instance) Run(ctx context.Context) error {
	if !i.heartbeat.Enabled {
		return nil
	}

	interval := time.Duration(i.heartbeat.Interval) * time.Second
	if interval <= 0 {
		return errors.New("heartbeat interval must be greater than zero")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := i.Step(ctx); err != nil {
				return err
			}
		}
	}
}

func (i *Instance) Step(ctx context.Context) error {
	item, ok := i.dequeuePrompt()
	if !ok {
		return nil
	}

	response, err := i.Prompt(ctx, item.prompt)
	if item.resultCh != nil {
		item.resultCh <- promptResult{response: response, err: err}
	}

	return err
}
