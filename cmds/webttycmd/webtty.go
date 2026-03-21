package webttycmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/coder/websocket"
	"github.com/creack/pty"
	"github.com/pubgo/redant"
)

const maxUploadBytes int64 = 100 << 20 // 100MB

func New() *redant.Command {
	var addr string
	var autoOpen bool

	return &redant.Command{
		Use:   "webtty",
		Short: "打开 WebTTY 交互终端页面",
		Long:  "启动最简 WebTTY：仅暴露本地 shell（WebSocket + PTY），不复用 webui。",
		Options: redant.OptionSet{
			{
				Flag:        "addr",
				Description: "WebTTY 服务监听地址",
				Value:       redant.StringOf(&addr),
				Default:     "127.0.0.1:18081",
			},
			{
				Flag:        "open",
				Description: "启动后自动打开浏览器",
				Value:       redant.BoolOf(&autoOpen),
				Default:     "true",
			},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			listenAddr := strings.TrimSpace(addr)
			if listenAddr == "" {
				listenAddr = "127.0.0.1:18081"
			}

			ln, err := net.Listen("tcp", listenAddr)
			if err != nil {
				return err
			}
			defer func() { _ = ln.Close() }()

			url := "http://" + ln.Addr().String()
			_, _ = fmt.Fprintf(inv.Stdout, "webtty listening on %s\n", url)
			_, _ = fmt.Fprintln(inv.Stdout, "tip: 页面会直接连接本地 shell")
			_, _ = fmt.Fprintln(inv.Stdout, "press Ctrl+C to stop")

			if autoOpen {
				if openErr := openBrowser(url); openErr != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "open browser failed: %v\n", openErr)
				}
			}

			server := &http.Server{Handler: newHandler()}
			errCh := make(chan error, 1)
			go func() {
				errCh <- server.Serve(ln)
			}()

			select {
			case serveErr := <-errCh:
				if errors.Is(serveErr, http.ErrServerClosed) {
					return nil
				}
				return serveErr
			case <-ctx.Done():
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = server.Shutdown(shutdownCtx)
				serveErr := <-errCh
				if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
					return serveErr
				}
				return nil
			}
		},
	}
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(indexHTML))
	})
	mux.HandleFunc("/ws", handleTerminalWS)
	mux.HandleFunc("/upload", handleUpload)
	return mux
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1024)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		http.Error(w, fmt.Sprintf("invalid multipart form: %v", err), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()

	fileName := sanitizeUploadFileName(header.Filename)
	if fileName == "" {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}

	wd, err := os.Getwd()
	if err != nil {
		http.Error(w, fmt.Sprintf("resolve working dir failed: %v", err), http.StatusInternalServerError)
		return
	}

	destDir, err := resolveUploadDir(wd, r.FormValue("dir"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		http.Error(w, fmt.Sprintf("create upload dir failed: %v", err), http.StatusInternalServerError)
		return
	}

	destPath := filepath.Join(destDir, fileName)
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		http.Error(w, fmt.Sprintf("open destination failed: %v", err), http.StatusInternalServerError)
		return
	}

	written, copyErr := io.Copy(out, file)
	closeErr := out.Close()
	if copyErr != nil {
		http.Error(w, fmt.Sprintf("write file failed: %v", copyErr), http.StatusInternalServerError)
		return
	}
	if closeErr != nil {
		http.Error(w, fmt.Sprintf("close file failed: %v", closeErr), http.StatusInternalServerError)
		return
	}

	relPath := fileName
	if rel, relErr := filepath.Rel(wd, destPath); relErr == nil {
		relPath = filepath.ToSlash(rel)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":        true,
		"savedPath": relPath,
		"size":      written,
	})
}

func sanitizeUploadFileName(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "" || base == "." || base == ".." {
		return ""
	}

	base = strings.ReplaceAll(base, "/", "_")
	base = strings.ReplaceAll(base, "\\", "_")
	base = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '_'
		}
		if r == ':' {
			return '_'
		}
		return r
	}, base)

	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == ".." {
		return ""
	}
	return base
}

func resolveUploadDir(wd, rawDir string) (string, error) {
	d := strings.TrimSpace(rawDir)
	if d == "" {
		return wd, nil
	}

	if filepath.IsAbs(d) {
		return "", fmt.Errorf("dir must be relative")
	}

	clean := filepath.Clean(d)
	if clean == "." {
		return wd, nil
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("dir cannot escape working directory")
	}

	return filepath.Join(wd, clean), nil
}

func handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()

	cols, rows := parseTerminalSize(r.URL.Query())

	ptmx, pts, err := pty.Open()
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, "open pty failed")
		return
	}
	defer func() {
		_ = ptmx.Close()
		_ = pts.Close()
	}()

	_ = pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})

	shellPath, shellArgs := detectInteractiveShell()
	cmd := exec.CommandContext(r.Context(), shellPath, shellArgs...)
	cmd.Stdin = pts
	cmd.Stdout = pts
	cmd.Stderr = pts
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	if wd, wdErr := os.Getwd(); wdErr == nil {
		cmd.Dir = wd
	}

	if err := cmd.Start(); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "start shell failed")
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var wg sync.WaitGroup
	var writeMu sync.Mutex

	writeWS := func(data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.Write(ctx, websocket.MessageText, data)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				if err := writeWS(buf[:n]); err != nil {
					cancel()
					return
				}
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) {
					cancel()
				}
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			msgType, data, readErr := conn.Read(ctx)
			if readErr != nil {
				cancel()
				return
			}
			if msgType != websocket.MessageText {
				continue
			}

			if strings.HasPrefix(string(data), "{") {
				var resizeMsg struct {
					Type string `json:"type"`
					Cols int    `json:"cols"`
					Rows int    `json:"rows"`
				}
				if json.Unmarshal(data, &resizeMsg) == nil && resizeMsg.Type == "resize" {
					if resizeMsg.Cols > 0 && resizeMsg.Rows > 0 {
						_ = pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(resizeMsg.Cols), Rows: uint16(resizeMsg.Rows)})
					}
					continue
				}
			}

			if len(data) > 0 {
				if _, err := ptmx.Write(data); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
	case <-waitCh:
		cancel()
	}

	wg.Wait()
}

func parseTerminalSize(values url.Values) (int, int) {
	cols := 80
	rows := 24
	if c, err := strconv.Atoi(strings.TrimSpace(values.Get("cols"))); err == nil && c > 0 && c <= 1000 {
		cols = c
	}
	if r, err := strconv.Atoi(strings.TrimSpace(values.Get("rows"))); err == nil && r > 0 && r <= 1000 {
		rows = r
	}
	return cols, rows
}

func detectInteractiveShell() (string, []string) {
	if runtime.GOOS == "windows" {
		if shell := strings.TrimSpace(os.Getenv("COMSPEC")); shell != "" {
			return shell, nil
		}
		return "cmd.exe", nil
	}

	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		return shell, []string{"-i"}
	}

	return "/bin/sh", []string{"-i"}
}

const indexHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>webtty</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@xterm/xterm@5.5.0/css/xterm.css" />
  <script src="https://cdn.jsdelivr.net/npm/@xterm/xterm@5.5.0/lib/xterm.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/@xterm/addon-fit@0.10.0/lib/addon-fit.js"></script>
  <style>
    html, body { margin: 0; padding: 0; width: 100%; height: 100%; background: #0b1220; color: #e2e8f0; font-family: system-ui, -apple-system, sans-serif; }
    #bar { height: 38px; display: flex; align-items: center; padding: 0 12px; border-bottom: 1px solid #1f2937; font-size: 13px; }
		#bar .spacer { flex: 1; }
		#uploadBtn { margin-left: 8px; border: 1px solid #334155; background: #0f172a; color: #e2e8f0; border-radius: 6px; padding: 4px 10px; cursor: pointer; }
		#uploadBtn:hover { background: #1e293b; }
		#uploadStatus { margin-left: 8px; color: #94a3b8; font-size: 12px; }
    #term { width: 100%; height: calc(100% - 39px); }
  </style>
</head>
<body>
	<div id="bar">
		<span>webtty: local shell</span>
		<span class="spacer"></span>
		<input id="uploadInput" type="file" hidden />
		<button id="uploadBtn" type="button">上传文件</button>
		<span id="uploadStatus"></span>
	</div>
  <div id="term"></div>
  <script>
    const term = new Terminal({ cursorBlink: true, fontSize: 13, theme: { background: '#020617', foreground: '#e2e8f0' } });
    const fit = new FitAddon.FitAddon();
    term.loadAddon(fit);
    term.open(document.getElementById('term'));
    fit.fit();

    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(proto + '//' + location.host + '/ws?cols=' + term.cols + '&rows=' + term.rows);
    ws.onmessage = (e) => term.write(e.data);
    ws.onclose = () => term.write('\r\n[disconnected]\r\n');
    ws.onerror = () => term.write('\r\n[ws error]\r\n');

		const uploadInput = document.getElementById('uploadInput');
		const uploadBtn = document.getElementById('uploadBtn');
		const uploadStatus = document.getElementById('uploadStatus');

		uploadBtn.addEventListener('click', () => uploadInput.click());

		uploadInput.addEventListener('change', async () => {
			const file = uploadInput.files && uploadInput.files[0];
			if (!file) return;

			uploadStatus.textContent = '上传中...';
			const form = new FormData();
			form.append('file', file);

			try {
				const resp = await fetch('/upload', { method: 'POST', body: form });
				const text = await resp.text();
				if (!resp.ok) {
					uploadStatus.textContent = '上传失败';
					term.write('\r\n[upload failed] ' + text + '\r\n');
					return;
				}

				const data = JSON.parse(text || '{}');
				const savedPath = String(data.savedPath || file.name);
				const size = Number(data.size || 0);
				uploadStatus.textContent = '已上传: ' + savedPath;
				term.write('\r\n[upload ok] ' + savedPath + ' (' + size + ' bytes)\r\n');
			} catch (err) {
				uploadStatus.textContent = '上传异常';
				term.write('\r\n[upload error] ' + String(err) + '\r\n');
			} finally {
				uploadInput.value = '';
			}
		});

    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(data);
    });

    term.onResize((size) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: size.cols, rows: size.rows }));
      }
    });

    window.addEventListener('resize', () => {
      fit.fit();
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
      }
    });
  </script>
</body>
</html>`

func AddWebTTYCommand(rootCmd *redant.Command) {
	rootCmd.Children = append(rootCmd.Children, New())
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}
