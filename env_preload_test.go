package redant

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseShortEFlag(t *testing.T) {
	tests := []struct {
		name           string
		arg            string
		wantValue      string
		wantInline     bool
		wantParsedFlag bool
	}{
		{name: "short without inline", arg: "-e", wantValue: "", wantInline: false, wantParsedFlag: true},
		{name: "short attached value", arg: "-eA=1", wantValue: "A=1", wantInline: true, wantParsedFlag: true},
		{name: "short equals value", arg: "-e=A=1", wantValue: "A=1", wantInline: true, wantParsedFlag: true},
		{name: "other short flag", arg: "-x", wantValue: "", wantInline: false, wantParsedFlag: false},
		{name: "long flag should ignore", arg: "--env", wantValue: "", wantInline: false, wantParsedFlag: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, inline, parsed := parseShortEFlag(tt.arg)
			if value != tt.wantValue || inline != tt.wantInline || parsed != tt.wantParsedFlag {
				t.Fatalf("got value=%q inline=%v parsed=%v, want value=%q inline=%v parsed=%v",
					value, inline, parsed,
					tt.wantValue, tt.wantInline, tt.wantParsedFlag,
				)
			}
		})
	}
}

func TestParseEnvFlagFromArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		index     int
		wantName  string
		wantValue string
		wantOK    bool
		wantErr   string
	}{
		{name: "long env next arg", args: []string{"--env", "A=1"}, index: 0, wantName: "env", wantValue: "A=1", wantOK: true},
		{name: "long env inline", args: []string{"--env=A=1"}, index: 0, wantName: "env", wantValue: "A=1", wantOK: true},
		{name: "short env next arg", args: []string{"-e", "A=1"}, index: 0, wantName: "env", wantValue: "A=1", wantOK: true},
		{name: "short env inline", args: []string{"-eA=1"}, index: 0, wantName: "env", wantValue: "A=1", wantOK: true},
		{name: "env file next arg", args: []string{"--env-file", ".env"}, index: 0, wantName: "env-file", wantValue: ".env", wantOK: true},
		{name: "env file inline", args: []string{"--env-file=.env,.env.local"}, index: 0, wantName: "env-file", wantValue: ".env,.env.local", wantOK: true},
		{name: "missing long env value", args: []string{"--env"}, index: 0, wantErr: "requires a value"},
		{name: "missing short env value", args: []string{"-e"}, index: 0, wantErr: "requires a value"},
		{name: "unknown flag", args: []string{"--name", "demo"}, index: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, value, ok, err := parseEnvFlagFromArgs(tt.args, tt.index)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err=%v, want contains %q", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if name != tt.wantName || value != tt.wantValue || ok != tt.wantOK {
				t.Fatalf("got name=%q value=%q ok=%v, want name=%q value=%q ok=%v",
					name, value, ok,
					tt.wantName, tt.wantValue, tt.wantOK,
				)
			}
		})
	}
}

func TestPreloadEnvFromArgs_AppliesAndRestores(t *testing.T) {
	const existing = "REDANT_PRELOAD_EXISTING"
	const created = "REDANT_PRELOAD_CREATED"

	t.Setenv(existing, "orig")
	_ = os.Unsetenv(created)

	restore, err := preloadEnvFromArgs([]string{"-e", existing + "=override", "--env", created + "=1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if restore == nil {
		t.Fatalf("restore func should not be nil")
	}

	if got := os.Getenv(existing); got != "override" {
		t.Fatalf("existing=%q, want override", got)
	}
	if got := os.Getenv(created); got != "1" {
		t.Fatalf("created=%q, want 1", got)
	}

	if err := restore(); err != nil {
		t.Fatalf("restore error: %v", err)
	}

	if got := os.Getenv(existing); got != "orig" {
		t.Fatalf("existing after restore=%q, want orig", got)
	}
	if _, ok := os.LookupEnv(created); ok {
		t.Fatalf("created should be unset after restore")
	}
}

func TestPreloadEnvFromArgs_EnvFileRepeatAndCSV(t *testing.T) {
	const inherited = "REDANT_PRELOAD_COMMON"
	const only1 = "REDANT_PRELOAD_FILE_A"
	const only2 = "REDANT_PRELOAD_FILE_B"

	t.Setenv(inherited, "orig-common")
	_ = os.Unsetenv(only1)
	_ = os.Unsetenv(only2)

	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "a.env")
	file2 := filepath.Join(tmpDir, "b.env")

	if err := os.WriteFile(file1, []byte(inherited+"=from-a\n"+only1+"=one\n"), 0o600); err != nil {
		t.Fatalf("write file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte("export "+inherited+"=\"from-b\"\n"+only2+"='two'\n"), 0o600); err != nil {
		t.Fatalf("write file2: %v", err)
	}

	restore, err := preloadEnvFromArgs([]string{"--env-file", file1, "--env-file", file2 + "," + file1})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if restore == nil {
		t.Fatalf("restore func should not be nil")
	}

	if got := os.Getenv(inherited); got != "from-a" {
		t.Fatalf("inherited=%q, want from-a", got)
	}
	if got := os.Getenv(only1); got != "one" {
		t.Fatalf("only1=%q, want one", got)
	}
	if got := os.Getenv(only2); got != "two" {
		t.Fatalf("only2=%q, want two", got)
	}

	if err := restore(); err != nil {
		t.Fatalf("restore error: %v", err)
	}

	if got := os.Getenv(inherited); got != "orig-common" {
		t.Fatalf("inherited after restore=%q, want orig-common", got)
	}
	if _, ok := os.LookupEnv(only1); ok {
		t.Fatalf("only1 should be unset after restore")
	}
	if _, ok := os.LookupEnv(only2); ok {
		t.Fatalf("only2 should be unset after restore")
	}
}

func TestPreloadEnvFromArgs_StopAtDoubleDash(t *testing.T) {
	const key = "REDANT_PRELOAD_STOP_AT_DASH"
	_ = os.Unsetenv(key)

	restore, err := preloadEnvFromArgs([]string{"--", "-e", key + "=1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if restore != nil {
		t.Fatalf("restore should be nil when no env is changed")
	}
	if _, ok := os.LookupEnv(key); ok {
		t.Fatalf("%s should not be set after --", key)
	}
}

func TestPreloadEnvFromArgs_RollbackOnError(t *testing.T) {
	const key = "REDANT_PRELOAD_ROLLBACK"
	_ = os.Unsetenv(key)

	restore, err := preloadEnvFromArgs([]string{"-e", key + "=1", "-e", "INVALID"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if restore != nil {
		t.Fatalf("restore should be nil on preload error")
	}
	if _, ok := os.LookupEnv(key); ok {
		t.Fatalf("%s should be rolled back when preload fails", key)
	}
}
