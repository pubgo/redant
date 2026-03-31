package redant

import (
	"context"
	"errors"
	"fmt"
)

// RunCallback runs invocation via original Run and dispatches typed callback.
//
// Callback will be invoked in two cases:
//   - unary response payload (from ResponseHandler)
//   - stream data payload (from ResponseStreamHandler)
func RunCallback[T any](inv *Invocation, callback func(T) error) error {
	if inv == nil {
		return errors.New("nil invocation")
	}
	if callback == nil {
		return errors.New("nil callback")
	}

	runCtx, cancel := context.WithCancel(inv.Context())
	defer cancel()
	runInv := inv.WithContext(runCtx)

	stream := runInv.ResponseStream()
	consumeErrCh := make(chan error, 1)
	go func() {
		defer close(consumeErrCh)
		for evt := range stream {
			typed, ok := evt.(T)
			if !ok {
				consumeErrCh <- fmt.Errorf("typed stream data mismatch: got %T", evt)
				cancel()
				return
			}

			if err := callback(typed); err != nil {
				consumeErrCh <- err
				cancel()
				return
			}
		}
	}()

	runErr := runInv.Run()
	cancel()

	var consumeErr error
	for err := range consumeErrCh {
		if err != nil {
			consumeErr = err
			break
		}
	}

	if consumeErr != nil {
		return errors.Join(runErr, consumeErr)
	}
	if runErr != nil {
		return runErr
	}

	resp, ok := runInv.Response()
	if !ok {
		return nil
	}

	typed, ok := resp.(T)
	if !ok {
		return fmt.Errorf("typed response mismatch: got %T", resp)
	}

	return callback(typed)
}
