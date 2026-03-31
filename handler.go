package redant

import (
	"context"
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
}

// NewInvocationStream creates a stream bound to invocation.
func NewInvocationStream(ctx context.Context, inv *Invocation) *InvocationStream {
	return &InvocationStream{
		ctx: ctx,
		inv: inv,
	}
}

// Send emits data to the invocation-owned response stream and mirrors
// text output to stdout, StreamError output to stderr.
func (s *InvocationStream) Send(data any) error {
	if s.inv != nil {
		if out := s.inv.responseStream; out != nil {
			select {
			case <-s.ctx.Done():
				return s.ctx.Err()
			case out <- data:
			}
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
	}
	return nil
}
