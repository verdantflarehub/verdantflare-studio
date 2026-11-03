package mcp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// Do not log err.Error(): transport errors commonly contain endpoints and
// protocol errors may include private response bodies. Only classify types.
func transportFailureReason(err error) string {
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
		return "timeout"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "dns"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "connection_refused"
	}
	if errors.Is(err, syscall.ECONNRESET) {
		return "connection_reset"
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "truncated"
	}
	return "transport_unknown"
}

func (g *Gateway) recordCallFailure(ctx context.Context, requestID, tool, stage, reason string, status int, started time.Time) {
	// Request IDs are correlation metadata, never arbitrary log text. Accept
	// only a canonical UUID, including from internal callers of callVersion.
	parsed, err := uuid.Parse(requestID)
	if err != nil || parsed.String() != requestID {
		requestID = "unavailable"
	}
	logger := g.diagnosticLogger
	if logger == nil {
		logger = slog.Default()
	}
	logger.WarnContext(ctx, "mcp downstream call failed",
		"request_id", requestID, "tool", tool, "stage", stage, "reason", reason,
		"downstream_status", status, "elapsed_ms", time.Since(started).Milliseconds())
}
