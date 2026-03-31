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

func eventKind(evt map[string]any) string {
	kind, _ := evt["event"].(string)
	if kind == "" {
		if _, ok := evt["error"]; ok {
			return "error"
		}
		return "output"
	}
	return kind
}

func eventTextForIO(evt map[string]any) (string, bool) {
	if rawErr, ok := evt["error"]; ok {
		switch v := rawErr.(type) {
		case *StreamError:
			if v != nil && v.Message != "" {
				return v.Message, true
			}
		case StreamError:
			if v.Message != "" {
				return v.Message, true
			}
		}
	}

	switch v := evt["data"].(type) {
	case string:
		return v, true
	case []byte:
		return string(v), true
	default:
		return "", false
	}
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

// Send emits a response event to invocation-owned response stream and mirrors
// text output to stdout/stderr.
func (s *InvocationStream) Send(msg map[string]any) error {
	if msg == nil {
		msg = map[string]any{"event": "output"}
	}
	if _, ok := msg["event"]; !ok {
		msg["event"] = eventKind(msg)
	}

	if s.inv != nil {
		if out := s.inv.responseStream; out != nil {
			select {
			case <-s.ctx.Done():
				return s.ctx.Err()
			case out <- msg:
			}
		}
	}

	if s.inv == nil {
		return nil
	}

	writer := s.inv.Stdout
	if eventKind(msg) == "error" {
		writer = s.inv.Stderr
	}

	if writer == nil {
		return nil
	}

	text, ok := eventTextForIO(msg)
	if !ok || text == "" {
		return nil
	}

	_, err := io.WriteString(writer, text)
	return err
}
