package redant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync/atomic"
	"time"
)

// HandlerFunc handles an Invocation of a command.
type HandlerFunc func(ctx context.Context, inv *Invocation) error

const defaultStreamResponseBuffer = 64

// StreamEnvelope is the NDJSON envelope written to stdout/stderr to distinguish
// structured response data from ordinary log output.
//
// Each envelope is a single JSON line: {"$":"resp","type":"T","data":...}
// Consumers can test for the "$" key to separate response payloads from
// plain-text log lines.
type StreamEnvelope struct {
	Kind string `json:"$"`              // "resp" or "error"
	Type string `json:"type,omitempty"` // type name from TypeInfo
	Data any    `json:"data"`           // payload
	Seq  *int64 `json:"seq,omitempty"`  // sequence number (stream only)
	Ts   *int64 `json:"ts,omitempty"`   // unix millis timestamp (stream only)
}

// StreamError models structured stream errors.
type StreamError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// ResponseTypeInfo describes runtime-visible output type metadata.
type ResponseTypeInfo struct {
	TypeName string         `json:"typeName,omitempty"`
	Schema   map[string]any `json:"schema,omitempty"`
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
	ctx      context.Context
	inv      *Invocation
	ch       chan any     // captured at creation to avoid racing with closeResponseStream
	typeName string       // response type name for envelope output
	seq      atomic.Int64 // stream sequence counter
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

// Send emits data to the invocation-owned response stream (channel) and mirrors
// structured output to stdout/stderr as NDJSON envelopes.
//
// Channel consumers (RunCallback, ResponseStream, WebUI stream WS) receive
// raw data without envelopes. Only the stdout/stderr mirror uses envelopes
// so that consumers can distinguish response payloads from ordinary log lines.
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

	seq := s.seq.Add(1) - 1 // 0-based
	ts := time.Now().UnixMilli()

	switch v := data.(type) {
	case *StreamError:
		if v != nil && s.inv.Stderr != nil {
			return writeEnvelope(s.inv.Stderr, StreamEnvelope{Kind: "error", Data: v, Seq: &seq, Ts: &ts})
		}
	case StreamError:
		if s.inv.Stderr != nil {
			return writeEnvelope(s.inv.Stderr, StreamEnvelope{Kind: "error", Data: v, Seq: &seq, Ts: &ts})
		}
	default:
		if s.inv.Stdout != nil {
			return writeEnvelope(s.inv.Stdout, StreamEnvelope{
				Kind: "resp",
				Type: s.typeName,
				Data: v,
				Seq:  &seq,
				Ts:   &ts,
			})
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
	return ResponseTypeInfo{
		TypeName: typeNameOf[T](),
		Schema:   reflectTypeSchema(reflect.TypeOf((*T)(nil)).Elem()),
	}
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
	return ResponseTypeInfo{
		TypeName: typeNameOf[T](),
		Schema:   reflectTypeSchema(reflect.TypeOf((*T)(nil)).Elem()),
	}
}

// adaptResponseHandler converts ResponseHandler to HandlerFunc.
func adaptResponseHandler(responseHandler ResponseHandler) HandlerFunc {
	if responseHandler == nil {
		return nil
	}

	typeName := responseHandler.TypeInfo().TypeName
	return func(ctx context.Context, inv *Invocation) error {
		resp, err := responseHandler.Handle(ctx, inv)
		if err != nil {
			return fmt.Errorf("running response handler: %w", err)
		}
		inv.setResponse(resp)
		return writeUnaryResponse(inv, resp, typeName)
	}
}

// adaptResponseStreamHandler converts ResponseStreamHandler to HandlerFunc.
func adaptResponseStreamHandler(responseStreamHandler ResponseStreamHandler) HandlerFunc {
	if responseStreamHandler == nil {
		return nil
	}

	typeName := responseStreamHandler.TypeInfo().TypeName
	return func(ctx context.Context, inv *Invocation) error {
		stream := NewInvocationStream(ctx, inv)
		stream.typeName = typeName
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

func writeUnaryResponse(inv *Invocation, resp any, typeName string) error {
	if inv == nil || inv.Stdout == nil || resp == nil {
		return nil
	}

	return writeEnvelope(inv.Stdout, StreamEnvelope{
		Kind: "resp",
		Type: typeName,
		Data: resp,
	})
}

// writeEnvelope writes a single NDJSON envelope line to w.
func writeEnvelope(w io.Writer, env StreamEnvelope) error {
	b, err := json.Marshal(env)
	if err != nil {
		_, werr := fmt.Fprintf(w, "%v", env.Data)
		return errors.Join(err, werr)
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
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

// reflectTypeSchema generates a JSON Schema (as map) from a Go reflect.Type.
// It handles struct fields (including json tags), primitives, slices, maps, and pointers.
func reflectTypeSchema(t reflect.Type) map[string]any {
	if t == nil {
		return nil
	}
	return reflectTypeSchemaInner(t, 0)
}

const maxSchemaDepth = 5

func reflectTypeSchemaInner(t reflect.Type, depth int) map[string]any {
	if depth > maxSchemaDepth {
		return map[string]any{"type": "object"}
	}

	// Dereference pointer.
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.String:
		return map[string]any{"type": "string"}

	case reflect.Slice, reflect.Array:
		return map[string]any{
			"type":  "array",
			"items": reflectTypeSchemaInner(t.Elem(), depth+1),
		}

	case reflect.Map:
		return map[string]any{
			"type":                 "object",
			"additionalProperties": reflectTypeSchemaInner(t.Elem(), depth+1),
		}

	case reflect.Struct:
		props := map[string]any{}
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name := f.Name
			omitempty := false
			if tag, ok := f.Tag.Lookup("json"); ok {
				parts := splitTag(tag)
				if parts[0] == "-" {
					continue
				}
				if parts[0] != "" {
					name = parts[0]
				}
				for _, p := range parts[1:] {
					if p == "omitempty" {
						omitempty = true
					}
				}
			}
			fieldSchema := reflectTypeSchemaInner(f.Type, depth+1)
			if omitempty {
				fieldSchema["x-omitempty"] = true
			}
			props[name] = fieldSchema
		}
		return map[string]any{
			"type":       "object",
			"properties": props,
		}

	case reflect.Interface:
		return map[string]any{}

	default:
		return map[string]any{"type": "string"}
	}
}

func splitTag(tag string) []string {
	var parts []string
	for tag != "" {
		i := 0
		for i < len(tag) && tag[i] != ',' {
			i++
		}
		parts = append(parts, tag[:i])
		if i < len(tag) {
			tag = tag[i+1:]
		} else {
			break
		}
	}
	return parts
}
