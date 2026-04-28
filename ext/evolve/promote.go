package evolve

import "os"

// AtomicSwap replaces dst with src using rename(2).
func AtomicSwap(src, dst string) error {
	return os.Rename(src, dst)
}
