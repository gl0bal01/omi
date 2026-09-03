package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gl0bal01/omi/internal/cmd"
)

var version = "0.3.0"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.ExecuteContext(ctx, version); err != nil {
		// Treat ctx.Err() as a clean cancel — exit without the noisy "context
		// canceled" line.
		if ctx.Err() != nil && errors.Is(err, context.Canceled) {
			os.Exit(130)
		}
		msg := err.Error()
		if !strings.HasPrefix(msg, "omi: ") {
			msg = "omi: " + msg
		}
		msg = cmd.SanitizeForTerminal(msg)
		_, _ = fmt.Fprintln(os.Stderr, msg)
		var ue *cmd.UsageError
		if errors.As(err, &ue) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
