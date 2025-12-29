package redant

import (
	_ "embed"
)

//go:embed .version/VERSION
var version string

func Version() string { return version }
