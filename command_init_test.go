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

	for _, flag := range []string{helpFlag, listCommandsFlag, listFlagsFlag, listFormatFlag, rawEnvelopeFlag, internalArgsOverrideFlag} {
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
			{Flag: listCommandsFlag, Description: "custom list-commands", Value: BoolOf(new(bool))},
		},
	}

	if err := root.init(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	count := 0
	for _, opt := range root.Options {
		if opt.Flag == listCommandsFlag {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected %s flag exactly once, got %d", listCommandsFlag, count)
	}
}
