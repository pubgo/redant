package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pubgo/redant"
)

// VersionInfo 展示 Unary 处理器的结构化响应类型。
type VersionInfo struct {
	Version   string `json:"version"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
}

func main() {
	versionCmd := &redant.Command{
		Use:   "version",
		Short: "返回版本信息（Unary ResponseHandler 示例）",
		ResponseHandler: redant.Unary(func(ctx context.Context, inv *redant.Invocation) (VersionInfo, error) {
			return VersionInfo{
				Version:   "1.2.3",
				BuildDate: time.Now().Format("2006-01-02"),
				GoVersion: "go1.23",
			}, nil
		}),
	}

	root := &redant.Command{
		Use:      "unary-demo",
		Short:    "Unary 响应处理器示例",
		Children: []*redant.Command{versionCmd},
	}

	// --- stdio 模式 ---
	fmt.Println("=== stdio 模式（JSON 自动输出到 stdout）===")
	inv := root.Invoke("version")
	inv.Stdout = os.Stdout
	inv.Stderr = os.Stderr
	if err := inv.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()

	// --- callback 模式 ---
	fmt.Println("=== callback 模式（RunCallback 泛型回调）===")
	inv2 := root.Invoke("version")
	inv2.Stdout = io.Discard
	inv2.Stderr = io.Discard
	if err := redant.RunCallback[VersionInfo](inv2, func(v VersionInfo) error {
		fmt.Printf("  Version:   %s\n", v.Version)
		fmt.Printf("  BuildDate: %s\n", v.BuildDate)
		fmt.Printf("  GoVersion: %s\n", v.GoVersion)
		return nil
	}); err != nil {
		fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("=== 完成 ===")
}
