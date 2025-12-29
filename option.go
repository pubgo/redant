package redant

import (
	"os"

	"github.com/spf13/pflag"
)

// Option is a configuration option for a CLI application.
type Option struct {
	// Flag is the long name of the flag used to configure this option. If unset,
	// flag configuring is disabled. This also serves as the option's identifier.
	Flag string `json:"flag,omitempty"`

	Description string `json:"description,omitempty"`

	// Required means this value must be set by some means. It requires
	// `ValueSourceType != ValueSourceNone`
	// If `Default` is set, then `Required` is ignored.
	Required bool `json:"required,omitempty"`

	// Shorthand is the one-character shorthand for the flag. If unset, no
	// shorthand is used.
	Shorthand string `json:"shorthand,omitempty"`

	// Envs is a list of environment variables used to configure this option.
	// The first non-empty environment variable value will be used.
	// If unset, environment configuring is disabled.
	Envs []string `json:"env,omitempty"`

	// Default is parsed into Value if set.
	Default string `json:"default,omitempty"`

	// Value includes the types listed in values.go.
	Value pflag.Value `json:"value,omitempty"`

	Hidden bool `json:"hidden,omitempty"`

	Deprecated string

	Category string
}

// OptionSet is a group of options that can be applied to a command.
type OptionSet []Option

// Add adds the given Options to the OptionSet.
func (optSet *OptionSet) Add(opts ...Option) {
	*optSet = append(*optSet, opts...)
}

// Filter will only return options that match the given filter. (return true)
func (optSet *OptionSet) Filter(filter func(opt Option) bool) OptionSet {
	cpy := make(OptionSet, 0)
	for _, opt := range *optSet {
		if filter(opt) {
			cpy = append(cpy, opt)
		}
	}
	return cpy
}

// Type returns the type of the option value
func (o Option) Type() string {
	if o.Value != nil {
		return o.Value.Type()
	}
	return "string"
}

func (optSet *OptionSet) FlagSet(name string) *pflag.FlagSet {
	if optSet == nil {
		return &pflag.FlagSet{}
	}

	fs := pflag.NewFlagSet(name, pflag.PanicOnError)
	for _, opt := range *optSet {
		if opt.Flag == "" {
			continue
		}

		var noOptDefValue string
		{
			no, ok := opt.Value.(NoOptDefValuer)
			if ok {
				noOptDefValue = no.NoOptDefValue()
			}
		}

		val := opt.Value
		if val == nil {
			val = DiscardValue
		}

		fs.AddFlag(&pflag.Flag{
			Name:        opt.Flag,
			Shorthand:   opt.Shorthand,
			Usage:       opt.Description,
			Value:       val,
			DefValue:    opt.Default,
			Changed:     false,
			Deprecated:  opt.Deprecated,
			NoOptDefVal: noOptDefValue,
			Hidden:      opt.Hidden,
		})
	}

	fs.Usage = func() {
		_, _ = os.Stderr.WriteString("Override (*FlagSet).Usage() to print help text.\n")
	}

	// Read environment variables and set flag values
	// Use the first non-empty environment variable value
	for _, opt := range *optSet {
		if opt.Flag == "" || opt.Value == nil {
			continue
		}

		// Try each environment variable in order, use the first non-empty one
		for _, envName := range opt.Envs {
			if envValue := os.Getenv(envName); envValue != "" {
				if flag := fs.Lookup(opt.Flag); flag != nil {
					if err := flag.Value.Set(envValue); err == nil {
						flag.Changed = true
						break // Use the first non-empty value
					}
				}
			}
		}
	}

	return fs
}
