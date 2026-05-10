package redant

import "testing"

func TestOptionInheritsToChildren(t *testing.T) {
	cases := []struct {
		name string
		opt  Option
		want bool
	}{
		{
			name: "default false with zero fields",
			opt:  Option{},
			want: false,
		},
		{
			name: "inherit explicit false disables inheritance",
			opt:  Option{Inherit: false},
			want: false,
		},
		{
			name: "inherit explicit true enables inheritance",
			opt:  Option{Inherit: true},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.opt.InheritsToChildren(); got != tc.want {
				t.Fatalf("InheritsToChildren() = %v, want %v", got, tc.want)
			}
		})
	}
}
