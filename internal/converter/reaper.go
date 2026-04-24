package converter

import (
	"context"
	"log/slog"
	"time"
)

// StartReaper launches a background loop that deletes expired converter
// rows every 10 minutes. Called once from server.New (or main).
//
// Why 10 minutes and not a shorter interval? Conversions are a few
// JSON blobs — nothing performance-critical. Every 10m is plenty to
// keep the table clean without holding a write-transaction open more
// often than needed. The DELETE itself uses the `expires_at` index so
// it's O(expired rows) rather than a full scan.
//
// The returned stop function cancels the loop and waits for the
// current tick to finish. Safe to call from graceful shutdown.
func StartReaper(ctx context.Context, repo *Repository, logger *slog.Logger) func() {
	rctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	interval := 10 * time.Minute

	go func() {
		defer close(done)
		// Immediate first sweep — covers the case where the service was
		// down for longer than `interval` and many rows are already
		// expired. Better to reap now than wait 10 more minutes.
		reap(rctx, repo, logger)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-rctx.Done():
				return
			case <-ticker.C:
				reap(rctx, repo, logger)
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

// reap runs one DELETE-expired pass and logs the count for ops visibility.
func reap(ctx context.Context, repo *Repository, logger *slog.Logger) {
	// Short per-pass timeout so a stuck pool doesn't leak the goroutine.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	n, err := repo.DeleteExpired(ctx)
	if err != nil {
		if logger != nil {
			logger.Warn("converter reaper: delete expired failed", "err", err)
		}
		return
	}
	if n > 0 && logger != nil {
		logger.Info("converter reaper: deleted expired jobs", "count", n)
	}
}
