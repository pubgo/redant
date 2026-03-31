package redant

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// HandlerFunc handles an Invocation of a command.
type HandlerFunc func(ctx context.Context, inv *Invocation) error

const defaultStreamResponseBuffer = 64

// StreamError models structured stream errors.
type StreamError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// InvocationStream provides response-stream communication.
// Response stream is internally created by invocation and automatically closed
// when response stream handling returns.
type InvocationStream struct {
	ctx context.Context
	inv *Invocation
	ch  chan any // captured at creation to avoid racing with closeResponseStream
}

// NewInvocationStream creates a stream bound to invocation.
func NewInvocationStream(ctx context.Context, inv *Invocation) *InvocationStream {
	var ch chan any
	if inv != nil {
		ch = inv.responseStream
	}
	return &InvocationStream{
		ctx: ctx,
		inv: inv,
		ch:  ch,
	}
}

// Send emits data to the invocation-owned response stream and mirrors
// text output to stdout, StreamError output to stderr.
func (s *InvocationStream) Send(data any) error {
	if s.ch != nil {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		case s.ch <- data:
		}
	}

	if s.inv == nil {
		return nil
	}

	switch v := data.(type) {
	case string:
		if s.inv.Stdout != nil {
			_, err := io.WriteString(s.inv.Stdout, v)
			return err
		}
	case []byte:
		if s.inv.Stdout != nil {
			_, err := s.inv.Stdout.Write(v)
			return err
		}
	case *StreamError:
		if v != nil && v.Message != "" && s.inv.Stderr != nil {
			_, err := io.WriteString(s.inv.Stderr, v.Message)
			return err
		}
	case StreamError:
		if v.Message != "" && s.inv.Stderr != nil {
			_, err := io.WriteString(s.inv.Stderr, v.Message)
			return err
		}
	default:
		if s.inv.Stdout != nil {
			b, err := json.Marshal(data)
			if err != nil {
				_, err = fmt.Fprintf(s.inv.Stdout, "%v", data)
				return err
			}
			_, err = s.inv.Stdout.Write(b)
			return err
		}
	}
	return nil
}
