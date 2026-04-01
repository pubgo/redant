package redant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
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

// ResponseTypeInfo describes runtime-visible output type metadata.
type ResponseTypeInfo struct {
	TypeName string `json:"typeName,omitempty"`
	Schema   string `json:"schema,omitempty"`
}

// ResponseHandler models request-response unary handling.
type ResponseHandler interface {
	Handle(context.Context, *Invocation) (any, error)
	TypeInfo() ResponseTypeInfo
}

// ResponseStreamHandler models request-response-stream handling.
type ResponseStreamHandler interface {
	HandleStream(context.Context, *Invocation, *InvocationStream) error
	TypeInfo() ResponseTypeInfo
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

// TypedWriter writes typed payloads to InvocationStream.
type TypedWriter[T any] struct {
	stream *InvocationStream
}

// Send emits a typed value to the stream.
func (w *TypedWriter[T]) Send(v T) error {
	return w.stream.Send(v)
}

// Raw returns the underlying InvocationStream for advanced use.
func (w *TypedWriter[T]) Raw() *InvocationStream {
	return w.stream
}

// Unary adapts a typed unary function into a runtime ResponseHandler.
func Unary[T any](fn func(context.Context, *Invocation) (T, error)) ResponseHandler {
	return unaryHandler[T]{fn: fn}
}

type unaryHandler[T any] struct {
	fn func(context.Context, *Invocation) (T, error)
}

func (h unaryHandler[T]) Handle(ctx context.Context, inv *Invocation) (any, error) {
	v, err := h.fn(ctx, inv)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (h unaryHandler[T]) TypeInfo() ResponseTypeInfo {
	return ResponseTypeInfo{TypeName: typeNameOf[T]()}
}

// Stream adapts typed stream producer into runtime ResponseStreamHandler.
func Stream[T any](fn func(context.Context, *Invocation, *TypedWriter[T]) error) ResponseStreamHandler {
	return streamHandler[T]{fn: fn}
}

type streamHandler[T any] struct {
	fn func(context.Context, *Invocation, *TypedWriter[T]) error
}

func (h streamHandler[T]) HandleStream(ctx context.Context, inv *Invocation, stream *InvocationStream) error {
	return h.fn(ctx, inv, &TypedWriter[T]{stream: stream})
}

func (h streamHandler[T]) TypeInfo() ResponseTypeInfo {
	return ResponseTypeInfo{TypeName: typeNameOf[T]()}
}

// AdaptResponseHandler converts ResponseHandler to legacy HandlerFunc.
func AdaptResponseHandler(responseHandler ResponseHandler) HandlerFunc {
	if responseHandler == nil {
		return nil
	}

	return func(ctx context.Context, inv *Invocation) error {
		resp, err := responseHandler.Handle(ctx, inv)
		if err != nil {
			return fmt.Errorf("running response handler: %w", err)
		}
		inv.setResponse(resp)
		return writeUnaryResponse(inv, resp)
	}
}

// AdaptResponseStreamHandler converts ResponseStreamHandler to legacy HandlerFunc.
func AdaptResponseStreamHandler(responseStreamHandler ResponseStreamHandler) HandlerFunc {
	if responseStreamHandler == nil {
		return nil
	}

	return func(ctx context.Context, inv *Invocation) error {
		stream := NewInvocationStream(ctx, inv)
		if err := responseStreamHandler.HandleStream(ctx, inv, stream); err != nil {
			return fmt.Errorf("running response stream handler: %w", err)
		}
		return nil
	}
}

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

func writeUnaryResponse(inv *Invocation, resp any) error {
	if inv == nil || inv.Stdout == nil || resp == nil {
		return nil
	}

	switch v := resp.(type) {
	case string:
		_, err := io.WriteString(inv.Stdout, v)
		return err
	case []byte:
		_, err := inv.Stdout.Write(v)
		return err
	default:
		b, err := json.Marshal(v)
		if err != nil {
			_, werr := io.WriteString(inv.Stdout, fmt.Sprintf("%v", resp))
			return errors.Join(err, werr)
		}
		_, err = inv.Stdout.Write(b)
		return err
	}
}

func typeNameOf[T any]() string {
	t := reflect.TypeOf((*T)(nil)).Elem()
	if t == nil {
		return "unknown"
	}
	if t.Name() == "" {
		return t.String()
	}
	if t.PkgPath() == "" {
		return t.String()
	}
	return t.PkgPath() + "." + t.Name()
}
