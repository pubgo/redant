package redant

import (
	"testing"
	"time"
)

func TestInt64Value(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"positive", "123", 123, false},
		{"negative", "-456", -456, false},
		{"zero", "0", 0, false},
		{"large", "9223372036854775807", 9223372036854775807, false},
		{"invalid", "abc", 0, true},
		{"float", "1.5", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v int64
			i := Int64Of(&v)

			err := i.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && v != tt.want {
				t.Errorf("value = %d, want %d", v, tt.want)
			}
		})
	}
}

func TestInt64Type(t *testing.T) {
	var v int64
	i := Int64Of(&v)

	if got := i.Type(); got != "int64" {
		t.Errorf("Type() = %q, want %q", got, "int64")
	}
}

func TestInt64String(t *testing.T) {
	var v int64 = 12345
	i := Int64Of(&v)

	if got := i.String(); got != "12345" {
		t.Errorf("String() = %q, want %q", got, "12345")
	}
}

func TestFloat64Value(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{"integer", "123", 123.0, false},
		{"decimal", "3.14159", 3.14159, false},
		{"negative", "-2.5", -2.5, false},
		{"scientific", "1e10", 1e10, false},
		{"invalid", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v float64
			f := Float64Of(&v)

			err := f.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && v != tt.want {
				t.Errorf("value = %f, want %f", v, tt.want)
			}
		})
	}
}

func TestBoolValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    bool
		wantErr bool
	}{
		{"true", "true", true, false},
		{"false", "false", false, false},
		{"1", "1", true, false},
		{"0", "0", false, false},
		{"empty", "", false, false},
		{"yes", "yes", false, true}, // strconv.ParseBool doesn't accept "yes"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v bool
			b := BoolOf(&v)

			err := b.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && v != tt.want {
				t.Errorf("value = %v, want %v", v, tt.want)
			}
		})
	}
}

func TestBoolNoOptDefValue(t *testing.T) {
	b := BoolOf(new(bool))
	if got := b.NoOptDefValue(); got != "true" {
		t.Errorf("NoOptDefValue() = %q, want %q", got, "true")
	}
}

func TestStringValue(t *testing.T) {
	var v string
	s := StringOf(&v)

	if err := s.Set("hello world"); err != nil {
		t.Errorf("Set() error = %v", err)
	}

	if v != "hello world" {
		t.Errorf("value = %q, want %q", v, "hello world")
	}

	if got := s.String(); got != "hello world" {
		t.Errorf("String() = %q, want %q", got, "hello world")
	}

	if got := s.Type(); got != "string" {
		t.Errorf("Type() = %q, want %q", got, "string")
	}
}

func TestStringArrayValue(t *testing.T) {
	tests := []struct {
		name    string
		inputs  []string
		want    []string
		wantErr bool
	}{
		{
			name:   "single value",
			inputs: []string{"a"},
			want:   []string{"a"},
		},
		{
			name:   "multiple values",
			inputs: []string{"a,b,c"},
			want:   []string{"a", "b", "c"},
		},
		{
			name:   "append",
			inputs: []string{"a", "b,c"},
			want:   []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v []string
			sa := StringArrayOf(&v)

			for _, input := range tt.inputs {
				if err := sa.Set(input); err != nil {
					t.Errorf("Set(%q) error = %v", input, err)
				}
			}

			if len(v) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(v), len(tt.want))
			}

			for i, val := range tt.want {
				if v[i] != val {
					t.Errorf("v[%d] = %q, want %q", i, v[i], val)
				}
			}
		})
	}
}

func TestDurationValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"seconds", "30s", 30 * time.Second, false},
		{"minutes", "5m", 5 * time.Minute, false},
		{"hours", "2h", 2 * time.Hour, false},
		{"mixed", "1h30m", 90 * time.Minute, false},
		{"invalid", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v time.Duration
			d := DurationOf(&v)

			err := d.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && v != tt.want {
				t.Errorf("value = %v, want %v", v, tt.want)
			}
		})
	}
}

func TestEnumValue(t *testing.T) {
	choices := []string{"debug", "info", "warn", "error"}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"valid lowercase", "debug", "debug", false},
		{"valid uppercase", "INFO", "INFO", false},
		{"invalid", "trace", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v string
			e := EnumOf(&v, choices...)

			err := e.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && v != tt.want {
				t.Errorf("value = %q, want %q", v, tt.want)
			}
		})
	}
}

func TestEnumArrayValue(t *testing.T) {
	choices := []string{"read", "write", "delete"}

	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{"single valid", "read", []string{"read"}, false},
		{"multiple valid", "read,write", []string{"read", "write"}, false},
		{"invalid", "execute", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v []string
			ea := EnumArrayOf(&v, choices...)

			err := ea.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(v) != len(tt.want) {
					t.Fatalf("len = %d, want %d", len(v), len(tt.want))
				}
				for i, val := range tt.want {
					if v[i] != val {
						t.Errorf("v[%d] = %q, want %q", i, v[i], val)
					}
				}
			}
		})
	}
}

func TestURLValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"http", "http://example.com", "http://example.com", false},
		{"https with path", "https://example.com/path", "https://example.com/path", false},
		{"with query", "http://example.com?q=1", "http://example.com?q=1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &URL{}

			err := u.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && u.String() != tt.want {
				t.Errorf("String() = %q, want %q", u.String(), tt.want)
			}
		})
	}
}

func TestHostPortValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHost string
		wantPort string
		wantErr  bool
	}{
		{"basic", "localhost:8080", "localhost", "8080", false},
		{"ipv4", "127.0.0.1:3000", "127.0.0.1", "3000", false},
		{"ipv6", "[::1]:8080", "::1", "8080", false},
		{"empty", "", "", "", true},
		{"no port", "localhost", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hp := &HostPort{}

			err := hp.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if hp.Host != tt.wantHost {
					t.Errorf("Host = %q, want %q", hp.Host, tt.wantHost)
				}
				if hp.Port != tt.wantPort {
					t.Errorf("Port = %q, want %q", hp.Port, tt.wantPort)
				}
			}
		})
	}
}

func TestValidatorWrapper(t *testing.T) {
	var port int64

	validator := Validate(Int64Of(&port), func(v *Int64) error {
		if v.Value() < 1 || v.Value() > 65535 {
			return &ValidationError{Field: "port", Message: "must be between 1 and 65535"}
		}
		return nil
	})

	// Valid port
	if err := validator.Set("8080"); err != nil {
		t.Errorf("Set(8080) error = %v", err)
	}
	if port != 8080 {
		t.Errorf("port = %d, want 8080", port)
	}

	// Invalid port
	if err := validator.Set("70000"); err == nil {
		t.Error("Set(70000) should return error")
	}

	// Type should delegate to underlying
	if got := validator.Type(); got != "int64" {
		t.Errorf("Type() = %q, want %q", got, "int64")
	}
}

// ValidationError is used in tests
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
