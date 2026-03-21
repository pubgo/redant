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
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/creack/pty"
	"github.com/pubgo/redant"
)

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
	return mux
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
    #term { width: 100%; height: calc(100% - 39px); }
  </style>
</head>
<body>
  <div id="bar">webtty: local shell</div>
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
