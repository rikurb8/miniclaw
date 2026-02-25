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
		case <-i.queueWakeChannel():
			// Process immediately when new work arrives.
			if err := i.processQueuedPrompts(ctx); err != nil {
				return err
			}
		case <-ticker.C:
			// Periodic draining is a safety net in case no wake signal is observed.
			if err := i.processQueuedPrompts(ctx); err != nil {
				return err
			}
		}
	}
}

func (i *Instance) processQueuedPrompts(ctx context.Context) error {
	for {
		item, ok := i.dequeuePrompt()
		if !ok {
			return nil
		}

		result, err := i.Prompt(ctx, item.prompt)
		if item.resultCh != nil {
			item.resultCh <- promptResult{result: result, err: err}
		}
		if err != nil {
			return err
		}
	}
}

func (i *Instance) Step(ctx context.Context) error {
	return i.processQueuedPrompts(ctx)
}
