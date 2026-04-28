package doccmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pubgo/redant"
)

func testRoot() *redant.Command {
	root := &redant.Command{
		Use:   "myapp",
		Short: "a test app",
	}
	root.Children = append(root.Children,
		&redant.Command{
			Use:   "greet",
			Short: "say hello",
			Options: redant.OptionSet{
				{
					Flag:        "name",
					Description: "who to greet",
					Default:     "world",
					Value:       redant.StringOf(new(string)),
				},
			},
			Args: redant.ArgSet{
				{Name: "message", Description: "greeting message", Required: true, Value: redant.StringOf(new(string))},
			},
			Handler: func(ctx context.Context, inv *redant.Invocation) error {
				return nil
			},
		},
		&redant.Command{
			Use:   "serve",
			Short: "start server",
			ResponseStreamHandler: redant.Stream(func(ctx context.Context, inv *redant.Invocation, out *redant.TypedWriter[string]) error {
				return nil
			}),
		},
		&redant.Command{
			Use:    "hidden",
			Short:  "hidden cmd",
			Hidden: true,
			Handler: func(ctx context.Context, inv *redant.Invocation) error {
				return nil
			},
		},
	)
	return root
}

func TestBuildCommandDoc(t *testing.T) {
	root := testRoot()
	doc := buildCommandDoc(root, "")

	t.Run("root_fields", func(t *testing.T) {
		if doc.Name != "myapp" {
			t.Fatalf("name = %q, want myapp", doc.Name)
		}
		if doc.Path != "myapp" {
			t.Fatalf("path = %q, want myapp", doc.Path)
		}
	})

	t.Run("children_exclude_hidden", func(t *testing.T) {
		for _, child := range doc.Children {
			if child.Name == "hidden" {
				t.Fatal("hidden command should not appear in doc tree")
			}
		}
	})

	t.Run("child_count", func(t *testing.T) {
		if len(doc.Children) != 2 {
			t.Fatalf("children = %d, want 2 (greet + serve)", len(doc.Children))
		}
	})

	t.Run("greet_has_args_and_options", func(t *testing.T) {
		var greetDoc *commandDoc
		for i := range doc.Children {
			if doc.Children[i].Name == "greet" {
				greetDoc = &doc.Children[i]
				break
			}
		}
		if greetDoc == nil {
			t.Fatal("greet command not found")
		}
		if len(greetDoc.Args) != 1 {
			t.Fatalf("greet args = %d, want 1", len(greetDoc.Args))
		}
		if greetDoc.Args[0].Name != "message" {
			t.Fatalf("arg name = %q, want message", greetDoc.Args[0].Name)
		}
		if len(greetDoc.Options) != 1 {
			t.Fatalf("greet options = %d, want 1", len(greetDoc.Options))
		}
		if greetDoc.Options[0].Flag != "name" {
			t.Fatalf("option flag = %q, want name", greetDoc.Options[0].Flag)
		}
		if greetDoc.HandlerType != "plain" {
			t.Fatalf("handler type = %q, want plain", greetDoc.HandlerType)
		}
	})

	t.Run("serve_is_stream", func(t *testing.T) {
		var serveDoc *commandDoc
		for i := range doc.Children {
			if doc.Children[i].Name == "serve" {
				serveDoc = &doc.Children[i]
				break
			}
		}
		if serveDoc == nil {
			t.Fatal("serve command not found")
		}
		if serveDoc.HandlerType != "stream" {
			t.Fatalf("serve handler type = %q, want stream", serveDoc.HandlerType)
		}
	})

	t.Run("nested_path", func(t *testing.T) {
		if doc.Children[0].Path != "myapp:greet" && doc.Children[1].Path != "myapp:greet" {
			t.Fatalf("expected colon-separated path for children")
		}
	})
}

func TestDocAppHandlers(t *testing.T) {
	root := testRoot()
	app := newDocApp(root)
	handler := app.handler()

	t.Run("api_tree_returns_json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/tree", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Fatalf("content-type = %q, want application/json", ct)
		}
		var tree map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &tree); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		if tree["name"] != "myapp" {
			t.Fatalf("tree name = %v, want myapp", tree["name"])
		}
	})

	t.Run("api_diagram_tree_returns_mermaid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/tree", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := w.Body.String()
		if !strings.HasPrefix(body, "graph TD") {
			t.Fatalf("expected Mermaid graph TD, got: %s", body[:min(len(body), 50)])
		}
	})

	t.Run("api_diagram_dispatch_returns_mermaid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/dispatch", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := w.Body.String()
		if !strings.HasPrefix(body, "flowchart TD") {
			t.Fatalf("expected Mermaid flowchart, got: %s", body[:min(len(body), 50)])
		}
	})

	t.Run("api_diagram_mcp_returns_mermaid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/mcp", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := w.Body.String()
		if !strings.HasPrefix(body, "sequenceDiagram") {
			t.Fatalf("expected Mermaid sequence, got: %s", body[:min(len(body), 50)])
		}
	})

	t.Run("index_returns_html", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/html") {
			t.Fatalf("content-type = %q, want text/html", ct)
		}
		body := w.Body.String()
		if !strings.Contains(body, "<!doctype html>") {
			t.Fatal("index should contain HTML doctype")
		}
		if !strings.Contains(body, "mermaid") {
			t.Fatal("index should reference mermaid")
		}
	})
}
