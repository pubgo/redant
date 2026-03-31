package redant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
)

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

// Unary adapts a typed unary function into a runtime ResponseHandler.
func Unary[T any](fn func(context.Context, *Invocation) (T, error)) ResponseHandler {
	return unaryHandler[T]{fn: fn}
}

// TypedWriter writes typed payloads to InvocationStream.
type TypedWriter[T any] struct {
	stream *InvocationStream
}

func (w *TypedWriter[T]) Output(v T) error {
	return w.stream.Send(map[string]any{"event": "output", "data": v})
}

func (w *TypedWriter[T]) OutputChunk(v T) error {
	return w.stream.Send(map[string]any{"event": "output_chunk", "data": v})
}

func (w *TypedWriter[T]) Raw() *InvocationStream {
	return w.stream
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

// Stream adapts typed stream producer into runtime ResponseStreamHandler.
func Stream[T any](fn func(context.Context, *Invocation, *TypedWriter[T]) error) ResponseStreamHandler {
	return streamHandler[T]{fn: fn}
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
