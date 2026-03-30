package redant

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
)

// HandlerFunc handles an Invocation of a command.
type HandlerFunc func(ctx context.Context, inv *Invocation) error

// StreamEventKind represents the direction/semantic of a stream message.
type StreamEventKind string

const streamJSONRPCVersion = "2.0"
const defaultStreamResponseBuffer = 64

const (
	StreamEventOutput  StreamEventKind = "output"
	StreamEventError   StreamEventKind = "error"
	StreamEventControl StreamEventKind = "control"
)

const (
	StreamMethodOutput      = "stream.output"
	StreamMethodOutputChunk = "stream.output.chunk"
	StreamMethodError       = "stream.error"
	StreamMethodControl     = "stream.control"
	StreamMethodRoundEnd    = "stream.round.end"
	StreamMethodExit        = "stream.exit"
)

// StreamMessage is the minimal envelope for structured stream events.
//
// New design (non-compatible):
//   - use JSON-RPC style envelope only: jsonrpc/id/method/type/data/error/meta
//   - no legacy Kind/Payload fields
//
// This schema is designed for command-as-web / command-as-mcp typed transport.
type StreamMessage struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      string            `json:"id,omitempty"`
	Method  string            `json:"method,omitempty"`
	Type    StreamEventKind   `json:"type,omitempty"`
	Round   int               `json:"round,omitempty"`
	Meta    map[string]string `json:"meta,omitempty"`
	Data    any               `json:"data,omitempty"`
	Error   *StreamError      `json:"error,omitempty"`
}

// StreamError models structured stream errors.
type StreamError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// StreamResponseType declares one stream response event contract.
type StreamResponseType struct {
	Method      string          `json:"method"`
	Type        StreamEventKind `json:"type"`
	Description string          `json:"description,omitempty"`
}

// Normalize fills default fields for the stream envelope.
func (m StreamMessage) Normalize() StreamMessage {
	if m.JSONRPC == "" {
		m.JSONRPC = streamJSONRPCVersion
	}

	if m.Type == "" && m.Error != nil {
		m.Type = StreamEventError
	}

	if m.Method == "" {
		switch m.Type {
		case StreamEventOutput:
			m.Method = StreamMethodOutput
		case StreamEventError:
			m.Method = StreamMethodError
		case StreamEventControl:
			m.Method = StreamMethodControl
		}
	}

	return m
}

func NewStreamOutput(data any) StreamMessage {
	return StreamMessage{Type: StreamEventOutput, Data: data}.Normalize()
}

func NewStreamOutputChunk(data any) StreamMessage {
	return StreamMessage{Type: StreamEventOutput, Method: StreamMethodOutputChunk, Data: data}.Normalize()
}

func NewStreamControl(data any) StreamMessage {
	return StreamMessage{Type: StreamEventControl, Data: data}.Normalize()
}

func NewStreamError(code int, message string, details any) StreamMessage {
	return StreamMessage{
		Type:   StreamEventError,
		Method: StreamMethodError,
		Error: &StreamError{
			Code:    code,
			Message: message,
			Details: details,
		},
	}.Normalize()
}

type StreamExit struct {
	Code     int    `json:"code"`
	Reason   string `json:"reason,omitempty"`
	TimedOut bool   `json:"timedOut,omitempty"`
}

func NewStreamExit(code int, reason string, timedOut bool, details any) StreamMessage {
	msg := StreamMessage{
		Type:   StreamEventControl,
		Method: StreamMethodExit,
		Data: StreamExit{
			Code:     code,
			Reason:   reason,
			TimedOut: timedOut,
		},
	}
	if details != nil {
		msg.Meta = map[string]string{"details": fmt.Sprintf("%v", details)}
	}
	return msg.Normalize()
}

func NewStreamRoundEnd(round int, reason string) StreamMessage {
	return StreamMessage{
		Type:   StreamEventControl,
		Method: StreamMethodRoundEnd,
		Round:  round,
		Data: map[string]any{
			"reason": reason,
		},
	}.Normalize()
}

func (m StreamMessage) Text() string {
	if m.Error != nil && m.Error.Message != "" {
		return m.Error.Message
	}

	switch v := m.Data.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

func (m StreamMessage) TextForIO() (string, bool) {
	if m.Error != nil && m.Error.Message != "" {
		return m.Error.Message, true
	}

	switch v := m.Data.(type) {
	case string:
		return v, true
	case []byte:
		return string(v), true
	default:
		return "", false
	}
}

// StreamHandlerFunc handles a command invocation and emits response stream events.
type StreamHandlerFunc func(ctx context.Context, stream *InvocationStream) error

// InvocationStream provides response-stream communication.
// Response stream is internally created by invocation and automatically closed
// when StreamHandler returns.
type InvocationStream struct {
	ctx context.Context
	inv *Invocation

	streamID string
	seq      atomic.Uint64
	roundSeq atomic.Uint64
}

// NewInvocationStream creates a stream bound to invocation.
func NewInvocationStream(ctx context.Context, inv *Invocation) *InvocationStream {
	streamID := "stream"
	if inv != nil && inv.Command != nil {
		name := strings.TrimSpace(inv.Command.FullName())
		if name != "" {
			streamID = strings.ReplaceAll(name, " ", ".")
		}
	}
	if inv != nil && inv.Annotations != nil {
		if v, ok := inv.Annotations["request_id"]; ok {
			if rid, ok := v.(string); ok && strings.TrimSpace(rid) != "" {
				streamID = strings.TrimSpace(rid)
			}
		}
	}

	return &InvocationStream{
		ctx:      ctx,
		inv:      inv,
		streamID: fmt.Sprintf("%s@%p", streamID, inv),
	}
}

func (s *InvocationStream) nextRoundID() int {
	return int(s.roundSeq.Add(1))
}

// EndRound emits a round-end control event.
func (s *InvocationStream) EndRound(reason string) error {
	return s.Send(NewStreamRoundEnd(s.nextRoundID(), reason))
}

func (s *InvocationStream) nextMessageID() string {
	n := s.seq.Add(1)
	return fmt.Sprintf("%s-%d", s.streamID, n)
}

// Invocation returns the underlying invocation context.
func (s *InvocationStream) Invocation() *Invocation {
	return s.inv
}

// Send emits a response event to invocation-owned response stream and mirrors
// text output to stdout/stderr.
func (s *InvocationStream) Send(msg StreamMessage) error {
	msg = msg.Normalize()
	if msg.ID == "" {
		msg.ID = s.nextMessageID()
	}

	if s.inv != nil {
		out := s.inv.ensureResponseStream(s.inv.responseBufferSize())
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		case out <- msg:
		}
	}

	if s.inv == nil {
		return nil
	}

	writer := s.inv.Stdout
	if msg.Type == StreamEventError || msg.Error != nil {
		writer = s.inv.Stderr
	}

	if writer == nil {
		return nil
	}

	text, ok := msg.TextForIO()
	if !ok || text == "" {
		return nil
	}

	_, err := io.WriteString(writer, text)
	return err
}

// Output sends an output text message.
func (s *InvocationStream) Output(text string) error {
	return s.Send(NewStreamOutput(text))
}

// OutputChunk sends a chunked output event.
func (s *InvocationStream) OutputChunk(text string) error {
	return s.Send(NewStreamOutputChunk(text))
}

// Outputf sends a formatted output message.
func (s *InvocationStream) Outputf(format string, args ...any) error {
	return s.Output(fmt.Sprintf(format, args...))
}

// Control sends a control message.
func (s *InvocationStream) Control(text string) error {
	return s.Send(NewStreamControl(text))
}

// Error sends a structured error message.
func (s *InvocationStream) Error(code int, message string, details any) error {
	return s.Send(NewStreamError(code, message, details))
}

// Exit sends an exit event with structured payload.
func (s *InvocationStream) Exit(code int, reason string, timedOut bool, details any) error {
	return s.Send(NewStreamExit(code, reason, timedOut, details))
}

// AdaptStreamHandler converts a StreamHandlerFunc into legacy HandlerFunc.
func AdaptStreamHandler(streamHandler StreamHandlerFunc) HandlerFunc {
	if streamHandler == nil {
		return nil
	}

	return func(ctx context.Context, inv *Invocation) error {
		inv.ensureResponseStream(inv.responseBufferSize())
		defer inv.closeResponseStream()

		stream := NewInvocationStream(ctx, inv)
		if err := streamHandler(ctx, stream); err != nil {
			return fmt.Errorf("running stream handler: %w", err)
		}
		return nil
	}
}
