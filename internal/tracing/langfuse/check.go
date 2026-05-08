package langfuse

import (
	"context"
	"time"
)

// CheckConnection sends a tiny synthetic trace through Langfuse ingestion.
// It is used by the settings UI to validate credentials without persisting
// them. The trace is intentionally tagged so it can be filtered out.
func CheckConnection(ctx context.Context, cfg Config) error {
	if cfg.FlushAt <= 0 {
		cfg.FlushAt = 1
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 10 * time.Second
	}
	cfg.Enabled = true
	if err := cfg.Validate(); err != nil {
		return err
	}
	c := newClient(cfg)
	now := isoTime(time.Now())
	return c.ingest(ctx, []ingestionEvent{
		{
			ID:        newID(),
			Timestamp: now,
			Type:      "trace-create",
			Body: traceBody{
				ID:        newID(),
				Timestamp: now,
				Name:      "weknora.langfuse.connection_check",
				Tags:      []string{"weknora", "connection-check"},
				Metadata: map[string]interface{}{
					"source": "weknora-settings",
				},
			},
		},
	})
}
