package webcmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/pubgo/redant"
	"github.com/pubgo/redant/internal/webui"
)

func New() *redant.Command {
	var addr string
	var autoOpen bool

	return &redant.Command{
		Use:   "web",
		Short: "打开可视化命令执行页面",
		Long:  "启动本地 Web 控制台：左侧命令列表，右侧 flags/args 输入，并展示完整调用过程与执行结果。",
		Options: redant.OptionSet{
			{
				Flag:        "addr",
				Description: "Web 服务监听地址",
				Value:       redant.StringOf(&addr),
				Default:     "127.0.0.1:18080",
			},
			{
				Flag:        "open",
				Description: "启动后自动打开浏览器",
				Value:       redant.BoolOf(&autoOpen),
				Default:     "true",
			},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			root := inv.Command
			for root.Parent() != nil {
				root = root.Parent()
			}

			listenAddr := strings.TrimSpace(addr)
			if listenAddr == "" {
				listenAddr = "127.0.0.1:18080"
			}

			ln, err := net.Listen("tcp", listenAddr)
			if err != nil {
				return err
			}
			defer func() { _ = ln.Close() }()

			url := "http://" + ln.Addr().String()
			_, _ = fmt.Fprintf(inv.Stdout, "web ui listening on %s\n", url)
			_, _ = fmt.Fprintf(inv.Stdout, "press Ctrl+C to stop\n")

			if autoOpen {
				if openErr := openBrowser(url); openErr != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "open browser failed: %v\n", openErr)
				}
			}

			server := &http.Server{Handler: webui.New(root).Handler()}
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

func AddWebCommand(rootCmd *redant.Command) {
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
