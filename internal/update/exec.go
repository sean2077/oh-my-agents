package update

import (
	"context"
	"os/exec"
	"time"
)

// execCommand runs the self-check subprocess with a hard timeout so a
// hung downloaded binary cannot wedge the updater.
func execCommand(path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, path, args...).Output()
}
