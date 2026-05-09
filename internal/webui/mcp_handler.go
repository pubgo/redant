package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pubgo/redant/internal/mcpclient"
)

// mcpSession returns a lazily-initialized in-process MCP client session.
// The session is reused across requests and recreated if closed.
func (a *App) mcpSession(ctx context.Context) (*mcpclient.Session, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.mcpSess != nil {
		return a.mcpSess, nil
	}

	sess, err := mcpclient.ConnectInProcess(ctx, a.root)
	if err != nil {
		return nil, err
	}
	a.mcpSess = sess
	return sess, nil
}

func (a *App) handleMCPInfo(w http.ResponseWriter, r *http.Request) {
	sess, err := a.mcpSession(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	info := sess.ServerInfo()
	writeJSON(w, http.StatusOK, info)
}

func (a *App) handleMCPTools(w http.ResponseWriter, r *http.Request) {
	sess, err := a.mcpSession(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	tools, err := sess.ListTools(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, tools)
}

func (a *App) handleMCPResources(w http.ResponseWriter, r *http.Request) {
	sess, err := a.mcpSession(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resources, err := sess.ListResources(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resources)
}

func (a *App) handleMCPPrompts(w http.ResponseWriter, r *http.Request) {
	sess, err := a.mcpSession(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	prompts, err := sess.ListPrompts(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, prompts)
}

type mcpCallRequest struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
}

func (a *App) handleMCPCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var req mcpCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON body: %v", err))
		return
	}

	if req.Tool == "" {
		writeJSONError(w, http.StatusBadRequest, "tool name is required")
		return
	}

	sess, err := a.mcpSession(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := sess.CallTool(ctx, req.Tool, req.Args)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type callResult struct {
		IsError           bool   `json:"isError"`
		Text              string `json:"text"`
		StructuredContent any    `json:"structuredContent,omitempty"`
	}

	writeJSON(w, http.StatusOK, callResult{
		IsError:           result.IsError,
		Text:              mcpclient.ToolResultText(result),
		StructuredContent: result.StructuredContent,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
