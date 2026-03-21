package webttycmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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

type uploadItemResponse struct {
	FileName  string `json:"fileName"`
	SavedPath string `json:"savedPath,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Error     string `json:"error,omitempty"`
}

type uploadResponse struct {
	OK    bool                 `json:"ok"`
	Dir   string               `json:"dir"`
	Items []uploadItemResponse `json:"items"`
}

type fileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	ModTime string `json:"modTime"`
}

type fileListResponse struct {
	OK      bool        `json:"ok"`
	Dir     string      `json:"dir"`
	Entries []fileEntry `json:"entries"`
}

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
	mux.HandleFunc("/api/files", handleListFiles)
	mux.HandleFunc("/download", handleDownload)
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

	headers := collectUploadFileHeaders(r.MultipartForm)
	if len(headers) == 0 {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}

	resp := uploadResponse{OK: true, Dir: filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(destDir, wd), string(filepath.Separator)))}
	if resp.Dir == "" {
		resp.Dir = "."
	}

	for _, header := range headers {
		item := uploadItemResponse{FileName: header.Filename}

		fileName := sanitizeUploadFileName(header.Filename)
		if fileName == "" {
			item.Error = "invalid filename"
			resp.OK = false
			resp.Items = append(resp.Items, item)
			continue
		}

		file, openErr := header.Open()
		if openErr != nil {
			item.Error = fmt.Sprintf("open upload failed: %v", openErr)
			resp.OK = false
			resp.Items = append(resp.Items, item)
			continue
		}

		destPath := filepath.Join(destDir, fileName)
		out, createErr := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if createErr != nil {
			_ = file.Close()
			item.Error = fmt.Sprintf("open destination failed: %v", createErr)
			resp.OK = false
			resp.Items = append(resp.Items, item)
			continue
		}

		written, copyErr := io.Copy(out, file)
		closeOutErr := out.Close()
		_ = file.Close()
		if copyErr != nil {
			item.Error = fmt.Sprintf("write file failed: %v", copyErr)
			resp.OK = false
			resp.Items = append(resp.Items, item)
			continue
		}
		if closeOutErr != nil {
			item.Error = fmt.Sprintf("close file failed: %v", closeOutErr)
			resp.OK = false
			resp.Items = append(resp.Items, item)
			continue
		}

		relPath := fileName
		if rel, relErr := filepath.Rel(wd, destPath); relErr == nil {
			relPath = filepath.ToSlash(rel)
		}

		item.SavedPath = relPath
		item.Size = written
		resp.Items = append(resp.Items, item)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func collectUploadFileHeaders(form *multipart.Form) []*multipart.FileHeader {
	if form == nil || len(form.File) == 0 {
		return nil
	}

	headers := make([]*multipart.FileHeader, 0)
	for _, files := range form.File {
		headers = append(headers, files...)
	}
	sort.SliceStable(headers, func(i, j int) bool {
		return headers[i].Filename < headers[j].Filename
	})
	return headers
}

func handleListFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wd, err := os.Getwd()
	if err != nil {
		http.Error(w, fmt.Sprintf("resolve working dir failed: %v", err), http.StatusInternalServerError)
		return
	}

	dirPath, err := resolveUploadDir(wd, r.URL.Query().Get("dir"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("read dir failed: %v", err), http.StatusBadRequest)
		return
	}

	out := make([]fileEntry, 0, len(entries))
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		fullPath := filepath.Join(dirPath, entry.Name())
		relPath := entry.Name()
		if rel, relErr := filepath.Rel(wd, fullPath); relErr == nil {
			relPath = filepath.ToSlash(rel)
		}
		out = append(out, fileEntry{
			Name:    entry.Name(),
			Path:    relPath,
			Size:    info.Size(),
			IsDir:   entry.IsDir(),
			ModTime: info.ModTime().Format(time.RFC3339),
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir && !out[j].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})

	relDir := "."
	if rel, relErr := filepath.Rel(wd, dirPath); relErr == nil {
		rel = filepath.ToSlash(rel)
		if rel != "." {
			relDir = rel
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fileListResponse{OK: true, Dir: relDir, Entries: out})
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wd, err := os.Getwd()
	if err != nil {
		http.Error(w, fmt.Sprintf("resolve working dir failed: %v", err), http.StatusInternalServerError)
		return
	}

	fullPath, relPath, err := resolveDownloadPath(wd, r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("stat file failed: %v", err), http.StatusBadRequest)
		return
	}
	if info.IsDir() {
		http.Error(w, "path is directory", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(relPath)))
	http.ServeFile(w, r, fullPath)
}

func resolveDownloadPath(wd, rawPath string) (string, string, error) {
	p := strings.TrimSpace(rawPath)
	if p == "" {
		return "", "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(p) {
		return "", "", fmt.Errorf("path must be relative")
	}

	clean := filepath.Clean(p)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path cannot escape working directory")
	}

	full := filepath.Join(wd, clean)
	return full, filepath.ToSlash(clean), nil
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

func writePTYInput(ptmx, pts *os.File, processPID int, data string) error {
	if ptmx == nil {
		return fmt.Errorf("pty file is nil")
	}
	if data == "" {
		return nil
	}

	if handled, err := trySignalFromControlInput(pts, processPID, data); handled {
		if err == nil {
			return nil
		}
		// fallback to raw write when signal forwarding fails
	}

	_, err := ptmx.Write([]byte(data))
	return err
}

func trySignalFromControlInput(pts *os.File, processPID int, data string) (bool, error) {
	b := []byte(data)
	if len(b) != 1 {
		return false, nil
	}

	switch b[0] {
	case 0x03: // Ctrl+C
		return true, signalFromControlInput(pts, processPID, "INT")
	case 0x1a: // Ctrl+Z
		return true, signalFromControlInput(pts, processPID, "TSTP")
	default:
		return false, nil
	}
}

func signalFromControlInput(pts *os.File, processPID int, signalName string) error {
	err := signalPTYForegroundProcessGroup(pts, signalName)
	if err == nil {
		return nil
	}

	if processPID <= 0 {
		return err
	}

	fallbackErr := signalProcessGroupByPID(processPID, signalName)
	if fallbackErr == nil {
		return nil
	}

	return errors.Join(err, fmt.Errorf("fallback process group signal failed: %w", fallbackErr))
}

func shellProcessPID(cmd *exec.Cmd) int {
	if cmd == nil || cmd.Process == nil {
		return 0
	}
	return cmd.Process.Pid
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
	prepareInteractiveShellCmd(cmd)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	if wd, wdErr := os.Getwd(); wdErr == nil {
		cmd.Dir = wd
	}

	if err := cmd.Start(); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "start shell failed")
		return
	}
	shellPID := shellProcessPID(cmd)

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
				if err := writePTYInput(ptmx, pts, shellPID, string(data)); err != nil {
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
		#layout { display: grid; grid-template-rows: 44px 1fr 180px; width: 100%; height: 100%; }
		#bar { display: flex; align-items: center; gap: 8px; padding: 0 12px; border-bottom: 1px solid #1f2937; font-size: 13px; }
		#bar .spacer { flex: 1; }
		.btn { border: 1px solid #334155; background: #0f172a; color: #e2e8f0; border-radius: 6px; padding: 4px 10px; cursor: pointer; }
		.btn:hover { background: #1e293b; }
		.input { border: 1px solid #334155; background: #020617; color: #e2e8f0; border-radius: 6px; padding: 4px 8px; font-size: 12px; }
		#uploadStatus { color: #94a3b8; font-size: 12px; white-space: nowrap; }
		#termWrap { position: relative; }
		#term { width: 100%; height: 100%; }
		#dropMask { position: absolute; inset: 0; display: none; align-items: center; justify-content: center; background: rgba(2, 6, 23, 0.75); color: #93c5fd; font-size: 16px; border: 2px dashed #3b82f6; pointer-events: none; z-index: 8; }
		#dropMask.show { display: flex; }
		#panel { border-top: 1px solid #1f2937; display: grid; grid-template-columns: 1fr 1fr; gap: 10px; padding: 8px 12px; overflow: hidden; }
		.card { border: 1px solid #1f2937; border-radius: 8px; padding: 8px; overflow: auto; background: #020617; }
		.card h4 { margin: 0 0 8px 0; font-size: 12px; color: #cbd5e1; }
		.row { display: flex; align-items: center; gap: 6px; margin-bottom: 6px; }
		.list { display: flex; flex-direction: column; gap: 6px; }
		.item { border: 1px solid #1e293b; border-radius: 6px; padding: 6px; font-size: 12px; }
		.progress { width: 100%; height: 6px; border-radius: 999px; background: #1e293b; overflow: hidden; margin-top: 4px; }
		.bar { height: 100%; width: 0%; background: #3b82f6; }
		.muted { color: #94a3b8; font-size: 11px; }
		.ok { color: #34d399; }
		.err { color: #f87171; }
		table { width: 100%; border-collapse: collapse; font-size: 12px; }
		th, td { text-align: left; border-bottom: 1px solid #1e293b; padding: 4px; }
		a { color: #93c5fd; text-decoration: none; }
	</style>
</head>
<body>
	<div id="layout">
		<div id="bar">
			<span>webtty: local shell</span>
			<span class="spacer"></span>
			<label class="muted" for="uploadDir">dir</label>
			<input id="uploadDir" class="input" placeholder="例如: tmp/artifacts" />
			<input id="uploadInput" type="file" multiple hidden />
			<button id="uploadBtn" class="btn" type="button" title="快捷键: Ctrl/Cmd + Shift + U">上传文件</button>
			<span id="uploadStatus"></span>
		</div>

		<div id="termWrap">
			<div id="dropMask">拖拽文件到此处上传</div>
			<div id="term"></div>
		</div>

		<div id="panel">
			<section class="card">
				<h4>上传队列（支持失败重试）</h4>
				<div id="uploadQueue" class="list"></div>
			</section>
			<section class="card">
				<div class="row">
					<h4 style="margin:0;">文件列表 / 下载</h4>
					<span class="spacer"></span>
					<button id="refreshFilesBtn" class="btn" type="button">刷新</button>
				</div>
				<table>
					<thead>
						<tr><th>name</th><th>size</th><th>action</th></tr>
					</thead>
					<tbody id="fileRows"></tbody>
				</table>
			</section>
		</div>
	</div>

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
		const uploadDir = document.getElementById('uploadDir');
		const uploadStatus = document.getElementById('uploadStatus');
		const uploadQueue = document.getElementById('uploadQueue');
		const fileRows = document.getElementById('fileRows');
		const refreshFilesBtn = document.getElementById('refreshFilesBtn');
		const dropMask = document.getElementById('dropMask');

		const failedMap = new Map();
		let seq = 0;

		function esc(s) {
			return String(s || '').replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;');
		}

		function setStatus(msg) {
			uploadStatus.textContent = msg || '';
		}

		function formatBytes(n) {
			const x = Number(n || 0);
			if (x < 1024) return x + ' B';
			if (x < 1024 * 1024) return (x / 1024).toFixed(1) + ' KB';
			if (x < 1024 * 1024 * 1024) return (x / (1024 * 1024)).toFixed(1) + ' MB';
			return (x / (1024 * 1024 * 1024)).toFixed(1) + ' GB';
		}

		function queueItemHTML(id, name) {
			return '<div class="item" id="up-' + id + '">' +
				'<div><strong>' + esc(name) + '</strong></div>' +
				'<div class="muted" id="up-status-' + id + '">等待上传...</div>' +
				'<div class="progress"><div class="bar" id="up-bar-' + id + '"></div></div>' +
				'<div id="up-actions-' + id + '" class="row" style="margin-top:4px;"></div>' +
			'</div>';
		}

		function setItemStatus(id, text, klass) {
			const el = document.getElementById('up-status-' + id);
			if (!el) return;
			el.className = klass ? klass : 'muted';
			el.textContent = text;
		}

		function setItemProgress(id, ratio) {
			const el = document.getElementById('up-bar-' + id);
			if (!el) return;
			const p = Math.max(0, Math.min(100, Math.floor(ratio * 100)));
			el.style.width = p + '%';
		}

		function setRetryAction(id, onRetry) {
			const box = document.getElementById('up-actions-' + id);
			if (!box) return;
			box.innerHTML = '';
			const btn = document.createElement('button');
			btn.type = 'button';
			btn.className = 'btn';
			btn.textContent = '重试';
			btn.addEventListener('click', onRetry);
			box.appendChild(btn);
		}

		function clearActions(id) {
			const box = document.getElementById('up-actions-' + id);
			if (box) box.innerHTML = '';
		}

		function uploadSingle(file, dir, id) {
			return new Promise((resolve, reject) => {
				const xhr = new XMLHttpRequest();
				const q = dir ? ('?dir=' + encodeURIComponent(dir)) : '';
				xhr.open('POST', '/upload' + q);

				xhr.upload.onprogress = (evt) => {
					if (!evt.lengthComputable) return;
					setItemProgress(id, evt.loaded / evt.total);
					setItemStatus(id, '上传中... ' + Math.floor((evt.loaded / evt.total) * 100) + '%', 'muted');
				};

				xhr.onerror = () => reject(new Error('network error'));
				xhr.onload = () => {
					if (xhr.status < 200 || xhr.status >= 300) {
						reject(new Error(xhr.responseText || ('http ' + xhr.status)));
						return;
					}
					let payload = null;
					try {
						payload = JSON.parse(xhr.responseText || '{}');
					} catch (_) {
						reject(new Error('invalid json response'));
						return;
					}

					const item = payload && Array.isArray(payload.items) ? payload.items[0] : null;
					if (!payload || !payload.ok || !item || item.error) {
						reject(new Error(item && item.error ? item.error : 'upload failed'));
						return;
					}

					resolve(item);
				};

				const form = new FormData();
				form.append('file', file);
				xhr.send(form);
			});
		}

		async function uploadFiles(files, dir) {
			const list = Array.from(files || []);
			if (list.length === 0) return;
			setStatus('准备上传 ' + list.length + ' 个文件...');

			let success = 0;
			let failed = 0;

			for (const file of list) {
				const id = ++seq;
				uploadQueue.insertAdjacentHTML('afterbegin', queueItemHTML(id, file.name));
				setItemProgress(id, 0.02);
				clearActions(id);

				try {
					const item = await uploadSingle(file, dir, id);
					setItemProgress(id, 1);
					setItemStatus(id, '上传成功: ' + item.savedPath + ' (' + formatBytes(item.size) + ')', 'ok');
					term.write('\r\n[upload ok] ' + item.savedPath + ' (' + item.size + ' bytes)\r\n');
					failedMap.delete(id);
					success += 1;
				} catch (err) {
					const reason = String(err && err.message ? err.message : err);
					setItemProgress(id, 1);
					setItemStatus(id, '上传失败: ' + reason, 'err');
					term.write('\r\n[upload failed] ' + file.name + ': ' + reason + '\r\n');
					failedMap.set(id, { file, dir });

					const retryHandler = async () => {
						const retry = failedMap.get(id);
						if (!retry) return;
						setItemProgress(id, 0.02);
						setItemStatus(id, '重试中...', 'muted');
						clearActions(id);
						try {
							const item = await uploadSingle(retry.file, retry.dir, id);
							setItemProgress(id, 1);
							setItemStatus(id, '重试成功: ' + item.savedPath + ' (' + formatBytes(item.size) + ')', 'ok');
							term.write('\r\n[upload retry ok] ' + item.savedPath + '\r\n');
							failedMap.delete(id);
							await refreshFiles();
						} catch (retryErr) {
							const retryReason = String(retryErr && retryErr.message ? retryErr.message : retryErr);
							setItemProgress(id, 1);
							setItemStatus(id, '重试失败: ' + retryReason, 'err');
							setRetryAction(id, retryHandler);
						}
					};
					setRetryAction(id, retryHandler);
					failed += 1;
				}
			}

			setStatus('完成: 成功 ' + success + ' / 失败 ' + failed);
			await refreshFiles();
		}

		async function refreshFiles() {
			const dir = String(uploadDir.value || '').trim();
			const q = dir ? ('?dir=' + encodeURIComponent(dir)) : '';
			try {
				const resp = await fetch('/api/files' + q);
				const text = await resp.text();
				if (!resp.ok) {
					fileRows.innerHTML = '<tr><td colspan="3" class="err">读取失败: ' + esc(text) + '</td></tr>';
					return;
				}
				const payload = JSON.parse(text || '{}');
				const entries = Array.isArray(payload.entries) ? payload.entries : [];
				if (entries.length === 0) {
					fileRows.innerHTML = '<tr><td colspan="3" class="muted">目录为空</td></tr>';
					return;
				}

				fileRows.innerHTML = entries.map((e) => {
					if (e.isDir) {
						return '<tr><td>📁 ' + esc(e.name) + '</td><td>-</td><td><button class="btn" data-enter="' + esc(e.path) + '">进入</button></td></tr>';
					}
					const dl = '/download?path=' + encodeURIComponent(e.path);
					return '<tr><td>' + esc(e.name) + '</td><td>' + esc(formatBytes(e.size)) + '</td><td><a href="' + dl + '">下载</a></td></tr>';
				}).join('');

				for (const btn of fileRows.querySelectorAll('button[data-enter]')) {
					btn.addEventListener('click', async () => {
						uploadDir.value = btn.getAttribute('data-enter') || '';
						await refreshFiles();
					});
				}
			} catch (err) {
				fileRows.innerHTML = '<tr><td colspan="3" class="err">读取异常: ' + esc(String(err)) + '</td></tr>';
			}
		}

		uploadBtn.addEventListener('click', () => uploadInput.click());

		document.addEventListener('keydown', (event) => {
			const target = event.target;
			if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) {
				return;
			}
			const key = String(event.key || '').toLowerCase();
			const hasPrimary = event.ctrlKey || event.metaKey;
			if (hasPrimary && event.shiftKey && !event.altKey && key === 'u') {
				event.preventDefault();
				uploadInput.click();
			}
		});

		uploadInput.addEventListener('change', async () => {
			const files = uploadInput.files;
			if (!files || files.length === 0) return;
			const dir = String(uploadDir.value || '').trim();
			await uploadFiles(files, dir);
			uploadInput.value = '';
		});

		const dragTargets = [document.body, document.getElementById('termWrap')];
		for (const el of dragTargets) {
			['dragenter', 'dragover'].forEach((ev) => {
				el.addEventListener(ev, (e) => {
					e.preventDefault();
					e.stopPropagation();
					dropMask.classList.add('show');
				});
			});
			['dragleave', 'drop'].forEach((ev) => {
				el.addEventListener(ev, (e) => {
					e.preventDefault();
					e.stopPropagation();
					if (ev === 'drop') {
						dropMask.classList.remove('show');
						const files = e.dataTransfer && e.dataTransfer.files;
						if (files && files.length > 0) {
							uploadFiles(files, String(uploadDir.value || '').trim());
						}
					} else {
						if (e.target === document.body || e.target === document.getElementById('termWrap')) {
							dropMask.classList.remove('show');
						}
					}
				});
			});
		}

		refreshFilesBtn.addEventListener('click', refreshFiles);

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

		refreshFiles();
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
