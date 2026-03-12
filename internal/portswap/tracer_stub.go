//go:build !(linux && amd64)

package portswap

import "errors"

// Run is not supported on non-Linux platforms.
func Run(cfg TracerConfig) Result {
	return Result{Error: errors.New("portswap: not supported on this platform")}
}
