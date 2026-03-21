package webttycmd

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pubgo/redant"
)

func TestAddWebTTYCommand(t *testing.T) {
	root := &redant.Command{Use: "app"}
	AddWebTTYCommand(root)

	if len(root.Children) != 1 {
		t.Fatalf("expected one child command, got %d", len(root.Children))
	}
	if root.Children[0].Name() != "webtty" {
		t.Fatalf("expected child command webtty, got %s", root.Children[0].Name())
	}
}

func TestWebTTYCommandRunAndShutdown(t *testing.T) {
	root := &redant.Command{Use: "app"}
	AddWebTTYCommand(root)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inv := root.Invoke("webtty", "--addr", "127.0.0.1:0", "--open=false")
	inv.Stdout = stdout
	inv.Stderr = stderr

	done := make(chan error, 1)
	go func() {
		done <- inv.WithContext(ctx).Run()
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("webtty command run failed: %v (stderr=%s)", err, stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("webtty command did not shutdown in time")
	}

	if !strings.Contains(stdout.String(), "webtty listening on") {
		t.Fatalf("expected startup output, got %q", stdout.String())
	}
}

func TestUploadEndpointSuccess(t *testing.T) {
	h := newHandler()
	ts := httptest.NewServer(h)
	defer ts.Close()

	tmp := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = fw.Write([]byte("hello webtty"))
	_ = mw.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/upload", &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		OK        bool   `json:"ok"`
		SavedPath string `json:"savedPath"`
		Size      int64  `json:"size"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.OK {
		t.Fatalf("expected ok=true")
	}
	if payload.SavedPath != "hello.txt" {
		t.Fatalf("unexpected savedPath: %q", payload.SavedPath)
	}

	data, err := os.ReadFile(filepath.Join(tmp, "hello.txt"))
	if err != nil {
		t.Fatalf("read uploaded file: %v", err)
	}
	if string(data) != "hello webtty" {
		t.Fatalf("unexpected file content: %q", string(data))
	}
}

func TestUploadEndpointRejectDirTraversal(t *testing.T) {
	h := newHandler()
	ts := httptest.NewServer(h)
	defer ts.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "evil.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = fw.Write([]byte("evil"))
	_ = mw.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/upload", &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	q := req.URL.Query()
	q.Set("dir", "../outside")
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
