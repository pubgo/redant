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

func TestParseLongFlag(t *testing.T) {
	tests := []struct {
		name       string
		arg        string
		wantName   string
		wantValue  string
		wantInline bool
		wantParsed bool
	}{
		{name: "plain long", arg: "--env", wantName: "env", wantValue: "", wantInline: false, wantParsed: true},
		{name: "long with equals", arg: "--env=A=1", wantName: "env", wantValue: "A=1", wantInline: true, wantParsed: true},
		{name: "single dash", arg: "-e", wantParsed: false},
		{name: "double dash only", arg: "--", wantParsed: false},
		{name: "non flag", arg: "env", wantParsed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, value, inline, parsed := parseLongFlag(tt.arg)
			if name != tt.wantName || value != tt.wantValue || inline != tt.wantInline || parsed != tt.wantParsed {
				t.Fatalf("got name=%q value=%q inline=%v parsed=%v, want name=%q value=%q inline=%v parsed=%v",
					name, value, inline, parsed,
					tt.wantName, tt.wantValue, tt.wantInline, tt.wantParsed,
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

func TestPreloadEnvFromArgs_ShortInlineEquals(t *testing.T) {
	const key = "REDANT_PRELOAD_SHORT_INLINE_EQUALS"
	_ = os.Unsetenv(key)

	restore, err := preloadEnvFromArgs([]string{"-e=" + key + "=ok"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if restore == nil {
		t.Fatalf("restore should not be nil")
	}
	if got := os.Getenv(key); got != "ok" {
		t.Fatalf("got %q, want ok", got)
	}
	if err := restore(); err != nil {
		t.Fatalf("restore error: %v", err)
	}
	if _, ok := os.LookupEnv(key); ok {
		t.Fatalf("%s should be unset after restore", key)
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

func TestPreloadEnvFromArgs_RollbackOnParseErrorAfterMutation(t *testing.T) {
	const key = "REDANT_PRELOAD_ROLLBACK_PARSE"
	_ = os.Unsetenv(key)

	restore, err := preloadEnvFromArgs([]string{"-e", key + "=1", "--env"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if restore != nil {
		t.Fatalf("restore should be nil on preload error")
	}
	if _, ok := os.LookupEnv(key); ok {
		t.Fatalf("%s should be rolled back when parse fails", key)
	}
}

func TestParseEnvAssignment(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantKey   string
		wantValue string
		wantErr   string
	}{
		{name: "basic", raw: "A=1", wantKey: "A", wantValue: "1"},
		{name: "trim spaces", raw: "  A = 1  ", wantKey: "A", wantValue: "1"},
		{name: "value includes equals", raw: "A=a=b", wantKey: "A", wantValue: "a=b"},
		{name: "empty raw", raw: "", wantErr: "empty environment assignment"},
		{name: "missing equals", raw: "A", wantErr: "expected KEY=VALUE"},
		{name: "missing key", raw: "=1", wantErr: "expected KEY=VALUE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, value, err := parseEnvAssignment(tt.raw)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err=%v, want contains %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if key != tt.wantKey || value != tt.wantValue {
				t.Fatalf("got key=%q value=%q, want key=%q value=%q", key, value, tt.wantKey, tt.wantValue)
			}
		})
	}
}

func TestApplyEnvAssignmentsCSV(t *testing.T) {
	got := make(map[string]string)
	setEnv := func(key, value string) error {
		got[key] = value
		return nil
	}

	err := applyEnvAssignmentsCSV(`A=1,"B=hello,world",C=3`, setEnv)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if got["A"] != "1" {
		t.Fatalf("A=%q, want 1", got["A"])
	}
	if got["B"] != "hello,world" {
		t.Fatalf("B=%q, want hello,world", got["B"])
	}
	if got["C"] != "3" {
		t.Fatalf("C=%q, want 3", got["C"])
	}
}

func TestLoadEnvFile_InvalidLineIncludesPosition(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(tmp, []byte("A=1\nINVALID\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	err := loadEnvFile(tmp, func(key, value string) error { return nil })
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), ":2:") {
		t.Fatalf("error should contain line number, got: %v", err)
	}
}

func TestConsumesNextArg(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		flagName string
		want     bool
	}{
		{name: "long env", current: "--env", flagName: "env", want: true},
		{name: "long env-file", current: "--env-file", flagName: "env-file", want: true},
		{name: "long inline", current: "--env=A=1", flagName: "env", want: false},
		{name: "short e", current: "-e", flagName: "env", want: true},
		{name: "short inline", current: "-eA=1", flagName: "env", want: false},
		{name: "other", current: "--name", flagName: "env", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := consumesNextArg(tt.current, tt.flagName); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPreloadEnvFromArgs_RollbackOnEnvFileLoadError(t *testing.T) {
	const key = "REDANT_PRELOAD_ROLLBACK_FILE_ERROR"
	_ = os.Unsetenv(key)

	missing := filepath.Join(t.TempDir(), "not-exists.env")
	restore, err := preloadEnvFromArgs([]string{"-e", key + "=1", "--env-file", missing})
	if err == nil {
		t.Fatalf("expected error")
	}
	if restore != nil {
		t.Fatalf("restore should be nil on preload error")
	}
	if _, ok := os.LookupEnv(key); ok {
		t.Fatalf("%s should be rolled back when env-file load fails", key)
	}
}

func TestPreloadEnvFromArgs_NoEnvFlagsNoChange(t *testing.T) {
	const key = "REDANT_PRELOAD_NO_CHANGE"
	t.Setenv(key, "orig")

	restore, err := preloadEnvFromArgs([]string{"--name", "demo", "subcmd"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if restore != nil {
		t.Fatalf("restore should be nil when env flags are absent")
	}
	if got := os.Getenv(key); got != "orig" {
		t.Fatalf("got %q, want orig", got)
	}
}

func TestNormalizeEnvValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "single quoted", in: "'abc'", want: "abc"},
		{name: "double quoted", in: "\"abc\"", want: "abc"},
		{name: "double quoted escape", in: "\"a\\nb\"", want: "a\nb"},
		{name: "invalid double quote keep raw", in: "\"abc", want: "\"abc"},
		{name: "trim spaces", in: "  abc  ", want: "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeEnvValue(tt.in); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRestoreEnvSnapshots(t *testing.T) {
	const existing = "REDANT_RESTORE_SNAPSHOT_EXISTING"
	const created = "REDANT_RESTORE_SNAPSHOT_CREATED"

	t.Setenv(existing, "current")
	_ = os.Unsetenv(created)

	if err := os.Setenv(created, "temp"); err != nil {
		t.Fatalf("set created: %v", err)
	}
	if err := os.Setenv(existing, "override"); err != nil {
		t.Fatalf("set existing: %v", err)
	}

	snapshots := map[string]envSnapshot{
		existing: {value: "orig", existed: true},
		created:  {value: "", existed: false},
	}

	if err := restoreEnvSnapshots(snapshots); err != nil {
		t.Fatalf("restoreEnvSnapshots err: %v", err)
	}

	if got := os.Getenv(existing); got != "orig" {
		t.Fatalf("existing=%q, want orig", got)
	}
	if _, ok := os.LookupEnv(created); ok {
		t.Fatalf("created should be unset")
	}
}
