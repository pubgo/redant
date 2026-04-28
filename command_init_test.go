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

	for _, flag := range []string{"help", "list-commands", "list-flags", "list-format", internalArgsOverrideFlag} {
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
			{Flag: "list-commands", Description: "custom list-commands", Value: BoolOf(new(bool))},
		},
	}

	if err := root.init(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	count := 0
	for _, opt := range root.Options {
		if opt.Flag == "list-commands" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected list-commands flag exactly once, got %d", count)
	}
}
