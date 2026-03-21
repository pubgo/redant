package webttycmd

import (
	"archive/zip"
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
	mux.HandleFunc("/download-zip", handleDownloadZip)
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

func handleDownloadZip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wd, err := os.Getwd()
	if err != nil {
		http.Error(w, fmt.Sprintf("resolve working dir failed: %v", err), http.StatusInternalServerError)
		return
	}

	dirPath, relDir, err := resolveDownloadDir(wd, r.URL.Query().Get("dir"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	info, err := os.Stat(dirPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("stat dir failed: %v", err), http.StatusBadRequest)
		return
	}
	if !info.IsDir() {
		http.Error(w, "dir is not a directory", http.StatusBadRequest)
		return
	}

	zipName := strings.ReplaceAll(relDir, "/", "_")
	if zipName == "." || zipName == "" {
		zipName = "workspace"
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", zipName+".zip"))

	zw := zip.NewWriter(w)
	defer func() { _ = zw.Close() }()

	err = filepath.WalkDir(dirPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		name := filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}

		if d.IsDir() {
			h := &zip.FileHeader{Name: name + "/", Method: zip.Store}
			h.SetMode(info.Mode())
			_, err = zw.CreateHeader(h)
			return err
		}

		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = name
		h.Method = zip.Deflate

		writer, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		_, err = io.Copy(writer, f)
		return err
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("zip directory failed: %v", err), http.StatusInternalServerError)
		return
	}
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

func resolveDownloadDir(wd, rawDir string) (string, string, error) {
	dirPath, err := resolveUploadDir(wd, rawDir)
	if err != nil {
		return "", "", err
	}

	relDir := "."
	if rel, relErr := filepath.Rel(wd, dirPath); relErr == nil {
		rel = filepath.ToSlash(rel)
		if rel != "." {
			relDir = rel
		}
	}

	return dirPath, relDir, nil
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
		.input-sm { width: 56px; }
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
		#totalProgress { margin-bottom: 8px; }
		#totalMeta { margin-top: 4px; }
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
			<label class="muted" for="uploadConcurrency">并发</label>
			<input id="uploadConcurrency" class="input input-sm" type="number" min="1" max="8" value="3" />
			<label class="muted" for="uploadSchedule">调度</label>
			<select id="uploadSchedule" class="input">
				<option value="fifo">FIFO</option>
				<option value="small-first">小文件优先</option>
				<option value="large-first">大文件优先</option>
			</select>
			<input id="uploadInput" type="file" multiple hidden />
			<button id="uploadBtn" class="btn" type="button" title="快捷键: Ctrl/Cmd + Shift + U">上传文件</button>
			<button id="cancelAllBtn" class="btn" type="button">取消全部</button>
			<button id="reconnectWsBtn" class="btn" type="button">重连会话</button>
			<span id="wsStatus" class="muted">ws: connecting</span>
			<span id="uploadStatus"></span>
		</div>

		<div id="termWrap">
			<div id="dropMask">拖拽文件到此处上传</div>
			<div id="term"></div>
		</div>

		<div id="panel">
			<section class="card">
				<h4>上传队列（支持失败重试）</h4>
				<div id="totalProgress" class="progress"><div id="totalBar" class="bar"></div></div>
				<div id="totalMeta" class="muted">总进度：0 / 0</div>
				<div id="uploadQueue" class="list"></div>
			</section>
			<section class="card">
				<div class="row">
					<h4 style="margin:0;">文件列表 / 下载</h4>
					<span class="spacer"></span>
					<button id="downloadDirZipBtn" class="btn" type="button">下载目录(zip)</button>
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
		let ws = null;
		let wsManualClose = false;
		let wsReconnectTimer = null;
		let wsReconnectAttempts = 0;

		const uploadInput = document.getElementById('uploadInput');
		const uploadBtn = document.getElementById('uploadBtn');
		const uploadDir = document.getElementById('uploadDir');
		const uploadConcurrency = document.getElementById('uploadConcurrency');
		const uploadSchedule = document.getElementById('uploadSchedule');
		const uploadStatus = document.getElementById('uploadStatus');
		const cancelAllBtn = document.getElementById('cancelAllBtn');
		const reconnectWsBtn = document.getElementById('reconnectWsBtn');
		const wsStatus = document.getElementById('wsStatus');
		const uploadQueue = document.getElementById('uploadQueue');
		const fileRows = document.getElementById('fileRows');
		const refreshFilesBtn = document.getElementById('refreshFilesBtn');
		const downloadDirZipBtn = document.getElementById('downloadDirZipBtn');
		const dropMask = document.getElementById('dropMask');
		const totalBar = document.getElementById('totalBar');
		const totalMeta = document.getElementById('totalMeta');

		const failedMap = new Map();
		const activeUploads = new Map();
		let currentBatch = null;
		let seq = 0;

		function esc(s) {
			return String(s || '').replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;');
		}

		function setStatus(msg) {
			uploadStatus.textContent = msg || '';
		}

		function getConcurrency() {
			const v = Number(uploadConcurrency.value || 3);
			if (!Number.isFinite(v)) return 3;
			return Math.max(1, Math.min(8, Math.floor(v)));
		}

		function getSchedule() {
			const s = String(uploadSchedule.value || 'fifo').trim();
			if (s === 'small-first' || s === 'large-first') return s;
			return 'fifo';
		}

		function setWSStatus(text, cls) {
			wsStatus.textContent = 'ws: ' + text;
			wsStatus.className = cls || 'muted';
		}

		function sendWS(data) {
			if (!ws || ws.readyState !== WebSocket.OPEN) return;
			ws.send(data);
		}

		function clearReconnectTimer() {
			if (wsReconnectTimer) {
				clearTimeout(wsReconnectTimer);
				wsReconnectTimer = null;
			}
		}

		function scheduleReconnect() {
			if (wsManualClose) return;
			if (wsReconnectTimer) return;
			if (wsReconnectAttempts >= 8) {
				setWSStatus('disconnected (max retries)', 'err');
				return;
			}

			const delay = Math.min(1000 * Math.pow(2, wsReconnectAttempts), 8000);
			wsReconnectAttempts += 1;
			setWSStatus('reconnecting in ' + delay + 'ms', 'muted');
			wsReconnectTimer = setTimeout(() => {
				wsReconnectTimer = null;
				connectTerminalWS(true);
			}, delay);
		}

		function connectTerminalWS(isRetry) {
			clearReconnectTimer();
			const url = proto + '//' + location.host + '/ws?cols=' + term.cols + '&rows=' + term.rows;
			try {
				ws = new WebSocket(url);
			} catch (err) {
				setWSStatus('connect error', 'err');
				scheduleReconnect();
				return;
			}

			setWSStatus(isRetry ? 'reconnecting...' : 'connecting...', 'muted');

			ws.onopen = () => {
				wsReconnectAttempts = 0;
				setWSStatus('connected', 'ok');
				if (isRetry) {
					term.write('\r\n[session reconnected]\r\n');
				}
				sendWS(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
			};

			ws.onmessage = (e) => term.write(e.data);

			ws.onclose = () => {
				if (wsManualClose) {
					setWSStatus('closed', 'muted');
					return;
				}
				setWSStatus('disconnected', 'err');
				term.write('\r\n[disconnected]\r\n');
				scheduleReconnect();
			};

			ws.onerror = () => {
				setWSStatus('error', 'err');
			};
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

		function setTotalProgress(done, total, success, failed, canceled, running) {
			const p = total > 0 ? Math.floor((done / total) * 100) : 0;
			totalBar.style.width = p + '%';
			totalMeta.textContent = '总进度：' + done + ' / ' + total + '（成功 ' + success + '，失败 ' + failed + '，取消 ' + canceled + '，进行中 ' + running + '）';
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

		function setCancelAction(id, onCancel) {
			const box = document.getElementById('up-actions-' + id);
			if (!box) return;
			box.innerHTML = '';
			const btn = document.createElement('button');
			btn.type = 'button';
			btn.className = 'btn';
			btn.textContent = '取消';
			btn.addEventListener('click', onCancel);
			box.appendChild(btn);
		}

		function clearActions(id) {
			const box = document.getElementById('up-actions-' + id);
			if (box) box.innerHTML = '';
		}

		function uploadSingle(file, dir, id, onProgress, onXhrReady) {
			return new Promise((resolve, reject) => {
				const xhr = new XMLHttpRequest();
				if (typeof onXhrReady === 'function') {
					onXhrReady(xhr);
				}
				const q = dir ? ('?dir=' + encodeURIComponent(dir)) : '';
				xhr.open('POST', '/upload' + q);

				xhr.upload.onprogress = (evt) => {
					if (!evt.lengthComputable) return;
					setItemProgress(id, evt.loaded / evt.total);
					setItemStatus(id, '上传中... ' + Math.floor((evt.loaded / evt.total) * 100) + '%', 'muted');
					if (typeof onProgress === 'function') {
						onProgress(evt.loaded, evt.total);
					}
				};

				xhr.onerror = () => reject(new Error('network error'));
				xhr.onabort = () => reject(new Error('canceled'));
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
			if (currentBatch && currentBatch.active) {
				setStatus('已有上传任务在执行，请先完成或取消全部');
				return;
			}
			const concurrency = getConcurrency();
			setStatus('准备上传 ' + list.length + ' 个文件（并发 ' + concurrency + '）...');

			let success = 0;
			let failed = 0;
			let canceled = 0;
			let done = 0;
			let running = 0;
			setTotalProgress(done, list.length, success, failed, canceled, running);

			const tasks = [];
			for (const file of list) {
				const id = ++seq;
				uploadQueue.insertAdjacentHTML('afterbegin', queueItemHTML(id, file.name));
				setItemProgress(id, 0.02);
				clearActions(id);
				tasks.push({ file, id, canceled: false, started: false, finished: false });
			}

			const schedule = getSchedule();
			if (schedule === 'small-first') {
				tasks.sort((a, b) => Number(a.file.size || 0) - Number(b.file.size || 0));
			} else if (schedule === 'large-first') {
				tasks.sort((a, b) => Number(b.file.size || 0) - Number(a.file.size || 0));
			}

			currentBatch = {
				active: true,
				tasks,
				cancelAll() {
					for (const t of tasks) {
						t.canceled = true;
						if (!t.started && !t.finished) {
							t.finished = true;
							done += 1;
							canceled += 1;
							setItemStatus(t.id, '已取消（未开始）', 'muted');
							setItemProgress(t.id, 1);
							clearActions(t.id);
						}
					}
					for (const [, xhr] of activeUploads.entries()) {
						try { xhr.abort(); } catch (_) { }
					}
					setTotalProgress(done, list.length, success, failed, canceled, running);
					setStatus('已请求取消全部上传任务');
				},
			};

			let cursor = 0;
			const runTask = async (task) => {
				if (task.canceled || task.finished) {
					return;
				}

				task.started = true;
				running += 1;
				setTotalProgress(done, list.length, success, failed, canceled, running);

				setCancelAction(task.id, () => {
					if (task.finished) return;
					task.canceled = true;
					const xhr = activeUploads.get(task.id);
					if (xhr) {
						try { xhr.abort(); } catch (_) { }
					} else {
						task.finished = true;
						done += 1;
						canceled += 1;
						setItemStatus(task.id, '已取消（排队中）', 'muted');
						setItemProgress(task.id, 1);
						clearActions(task.id);
						setTotalProgress(done, list.length, success, failed, canceled, running);
					}
				});

				try {
					const item = await uploadSingle(task.file, dir, task.id, null, (xhr) => {
						activeUploads.set(task.id, xhr);
					});
					if (task.canceled) {
						task.finished = true;
						canceled += 1;
						setItemStatus(task.id, '已取消', 'muted');
						setItemProgress(task.id, 1);
						clearActions(task.id);
						return;
					}

					task.finished = true;
					setItemProgress(task.id, 1);
					setItemStatus(task.id, '上传成功: ' + item.savedPath + ' (' + formatBytes(item.size) + ')', 'ok');
					clearActions(task.id);
					term.write('\r\n[upload ok] ' + item.savedPath + ' (' + item.size + ' bytes)\r\n');
					failedMap.delete(task.id);
					success += 1;
				} catch (err) {
					const reason = String(err && err.message ? err.message : err);
					if (reason === 'canceled' || task.canceled) {
						task.finished = true;
						setItemProgress(task.id, 1);
						setItemStatus(task.id, '已取消', 'muted');
						clearActions(task.id);
						canceled += 1;
						setTotalProgress(done, list.length, success, failed, canceled, running);
						return;
					}

					task.finished = true;
					setItemProgress(task.id, 1);
					setItemStatus(task.id, '上传失败: ' + reason, 'err');
					term.write('\r\n[upload failed] ' + task.file.name + ': ' + reason + '\r\n');
					failedMap.set(task.id, { file: task.file, dir });
					failed += 1;
					setTotalProgress(done, list.length, success, failed, canceled, running);

					const retryHandler = async () => {
						const retry = failedMap.get(task.id);
						if (!retry) return;
						if (task.canceled) return;
						running += 1;
						setTotalProgress(done, list.length, success, failed, canceled, running);
						setItemProgress(task.id, 0.02);
						setItemStatus(task.id, '重试中...', 'muted');
						clearActions(task.id);
						setCancelAction(task.id, () => {
							task.canceled = true;
							const xhr = activeUploads.get(task.id);
							if (xhr) {
								try { xhr.abort(); } catch (_) { }
							}
						});
						try {
							const item = await uploadSingle(retry.file, retry.dir, task.id, null, (xhr) => {
								activeUploads.set(task.id, xhr);
							});
							if (task.canceled) {
								setItemProgress(task.id, 1);
								setItemStatus(task.id, '已取消', 'muted');
								clearActions(task.id);
								canceled += 1;
								running = Math.max(0, running - 1);
								setTotalProgress(done, list.length, success, failed, canceled, running);
								return;
							}
							setItemProgress(task.id, 1);
							setItemStatus(task.id, '重试成功: ' + item.savedPath + ' (' + formatBytes(item.size) + ')', 'ok');
							clearActions(task.id);
							term.write('\r\n[upload retry ok] ' + item.savedPath + '\r\n');
							failedMap.delete(task.id);
							success += 1;
							failed = Math.max(0, failed - 1);
							running = Math.max(0, running - 1);
							setTotalProgress(done, list.length, success, failed, canceled, running);
							await refreshFiles();
						} catch (retryErr) {
							const retryReason = String(retryErr && retryErr.message ? retryErr.message : retryErr);
							if (retryReason === 'canceled' || task.canceled) {
								setItemProgress(task.id, 1);
								setItemStatus(task.id, '已取消', 'muted');
								clearActions(task.id);
								canceled += 1;
								running = Math.max(0, running - 1);
								setTotalProgress(done, list.length, success, failed, canceled, running);
								return;
							}
							setItemProgress(task.id, 1);
							setItemStatus(task.id, '重试失败: ' + retryReason, 'err');
							running = Math.max(0, running - 1);
							setTotalProgress(done, list.length, success, failed, canceled, running);
							setRetryAction(task.id, retryHandler);
						}
					};
					setRetryAction(task.id, retryHandler);
				} finally {
					activeUploads.delete(task.id);
					done += 1;
					running = Math.max(0, running - 1);
					setTotalProgress(done, list.length, success, failed, canceled, running);
				}
			};

			const worker = async () => {
				while (true) {
					const idx = cursor;
					cursor += 1;
					if (idx >= tasks.length) {
						return;
					}
					await runTask(tasks[idx]);
				}
			};

			const workerCount = Math.min(concurrency, tasks.length);
			await Promise.all(Array.from({ length: workerCount }, () => worker()));
			if (currentBatch) {
				currentBatch.active = false;
			}

			setStatus('完成: 成功 ' + success + ' / 失败 ' + failed + ' / 取消 ' + canceled);
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

		cancelAllBtn.addEventListener('click', () => {
			if (currentBatch && currentBatch.active) {
				currentBatch.cancelAll();
				return;
			}
			setStatus('当前没有可取消的上传任务');
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

		downloadDirZipBtn.addEventListener('click', () => {
			const dir = String(uploadDir.value || '').trim();
			const q = dir ? ('?dir=' + encodeURIComponent(dir)) : '';
			window.location.href = '/download-zip' + q;
		});

		term.onData((data) => {
			sendWS(data);
		});

		term.onResize((size) => {
			sendWS(JSON.stringify({ type: 'resize', cols: size.cols, rows: size.rows }));
		});

		window.addEventListener('resize', () => {
			fit.fit();
			sendWS(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
		});

		reconnectWsBtn.addEventListener('click', () => {
			wsManualClose = false;
			if (ws && ws.readyState === WebSocket.OPEN) {
				try { ws.close(); } catch (_) { }
			}
			connectTerminalWS(true);
		});

		window.addEventListener('beforeunload', () => {
			wsManualClose = true;
			clearReconnectTimer();
			if (ws && ws.readyState === WebSocket.OPEN) {
				try { ws.close(); } catch (_) { }
			}
		});

		connectTerminalWS(false);
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
