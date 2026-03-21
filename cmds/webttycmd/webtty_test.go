package webttycmd

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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

	var payload uploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.OK {
		t.Fatalf("expected ok=true")
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected one item, got %d", len(payload.Items))
	}
	if payload.Items[0].SavedPath != "hello.txt" {
		t.Fatalf("unexpected savedPath: %q", payload.Items[0].SavedPath)
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

func TestUploadEndpointMultiFilesToSubDir(t *testing.T) {
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
	fw1, err := mw.CreateFormFile("file", "a.txt")
	if err != nil {
		t.Fatalf("create form file a: %v", err)
	}
	_, _ = fw1.Write([]byte("A"))
	fw2, err := mw.CreateFormFile("file", "b.txt")
	if err != nil {
		t.Fatalf("create form file b: %v", err)
	}
	_, _ = fw2.Write([]byte("BB"))
	_ = mw.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/upload?dir=uploads/sub", &body)
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

	var payload uploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.OK {
		t.Fatalf("expected ok=true, got %+v", payload)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("expected 2 uploaded items, got %d", len(payload.Items))
	}

	if _, err := os.Stat(filepath.Join(tmp, "uploads", "sub", "a.txt")); err != nil {
		t.Fatalf("a.txt missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "uploads", "sub", "b.txt")); err != nil {
		t.Fatalf("b.txt missing: %v", err)
	}
}

func TestListAndDownloadEndpoints(t *testing.T) {
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

	if err := os.MkdirAll(filepath.Join(tmp, "downloads"), 0o755); err != nil {
		t.Fatalf("mkdir downloads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "downloads", "x.txt"), []byte("download-me"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	listResp, err := http.Get(ts.URL + "/api/files?dir=downloads")
	if err != nil {
		t.Fatalf("list files request: %v", err)
	}
	defer func() { _ = listResp.Body.Close() }()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected list 200, got %d", listResp.StatusCode)
	}

	var listPayload fileListResponse
	if err := json.NewDecoder(listResp.Body).Decode(&listPayload); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if !listPayload.OK {
		t.Fatalf("expected list ok=true")
	}
	if len(listPayload.Entries) == 0 {
		t.Fatalf("expected list entries")
	}

	dlResp, err := http.Get(ts.URL + "/download?path=downloads/x.txt")
	if err != nil {
		t.Fatalf("download request: %v", err)
	}
	defer func() { _ = dlResp.Body.Close() }()
	if dlResp.StatusCode != http.StatusOK {
		t.Fatalf("expected download 200, got %d", dlResp.StatusCode)
	}

	content, err := io.ReadAll(dlResp.Body)
	if err != nil {
		t.Fatalf("read download body: %v", err)
	}
	if string(content) != "download-me" {
		t.Fatalf("unexpected download content: %q", string(content))
	}
}

func TestDownloadZipEndpoint(t *testing.T) {
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

	if err := os.MkdirAll(filepath.Join(tmp, "pack", "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "pack", "a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "pack", "sub", "b.txt"), []byte("BB"), 0o644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}

	resp, err := http.Get(ts.URL + "/download-zip?dir=pack")
	if err != nil {
		t.Fatalf("download zip request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read zip body: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}

	seen := map[string]bool{}
	for _, f := range zr.File {
		seen[f.Name] = true
	}

	if !seen["a.txt"] {
		t.Fatalf("zip missing a.txt, got %+v", seen)
	}
	if !seen["sub/b.txt"] {
		t.Fatalf("zip missing sub/b.txt, got %+v", seen)
	}
}

func TestTrySignalFromControlInput(t *testing.T) {
	t.Run("ctrl+c recognized", func(t *testing.T) {
		handled, err := trySignalFromControlInput(nil, 0, "\x03")
		if !handled {
			t.Fatalf("expected handled=true for ctrl+c")
		}
		if err == nil {
			t.Fatalf("expected non-nil error when pty/process unavailable")
		}
	})

	t.Run("non-control ignored", func(t *testing.T) {
		handled, err := trySignalFromControlInput(nil, 0, "a")
		if handled {
			t.Fatalf("expected handled=false for normal input")
		}
		if err != nil {
			t.Fatalf("expected nil error for normal input, got %v", err)
		}
	})

	t.Run("multi-byte ignored", func(t *testing.T) {
		handled, err := trySignalFromControlInput(nil, 0, "ab")
		if handled {
			t.Fatalf("expected handled=false for multi-byte input")
		}
		if err != nil {
			t.Fatalf("expected nil error for multi-byte input, got %v", err)
		}
	})
}

func TestShellProcessPID(t *testing.T) {
	if got := shellProcessPID(nil); got != 0 {
		t.Fatalf("expected 0 pid for nil cmd, got %d", got)
	}

	cmd := &exec.Cmd{}
	if got := shellProcessPID(cmd); got != 0 {
		t.Fatalf("expected 0 pid for cmd without process, got %d", got)
	}
}
