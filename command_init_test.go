package redant

import "testing"

func TestCommandInitIsIdempotentForGlobalFlags(t *testing.T) {
	root := &Command{Use: "app"}

	if err := root.init(); err != nil {
		t.Fatalf("first init failed: %v", err)
	}
	if err := root.init(); err != nil {
		t.Fatalf("second init failed: %v", err)
	}

	counts := map[string]int{}
	for _, opt := range root.Options {
		if opt.Flag == "" {
			continue
		}
		counts[opt.Flag]++
	}

	for _, flag := range []string{"help", "list-commands", "list-flags", "env", "env-file", internalArgsOverrideFlag} {
		if counts[flag] != 1 {
			t.Fatalf("expected global flag %q exactly once, got %d", flag, counts[flag])
		}
	}

	globals := root.GetGlobalFlags()
	_ = globals.FlagSet(root.Name())
}

func TestCommandInitDoesNotOverrideExistingRootGlobalFlag(t *testing.T) {
	root := &Command{
		Use: "app",
		Options: OptionSet{
			{Flag: "env", Description: "custom env", Value: StringArrayOf(new([]string))},
		},
	}

	if err := root.init(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	envCount := 0
	for _, opt := range root.Options {
		if opt.Flag == "env" {
			envCount++
		}
	}
	if envCount != 1 {
		t.Fatalf("expected env flag exactly once, got %d", envCount)
	}
}
