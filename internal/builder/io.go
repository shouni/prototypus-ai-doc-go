package builder

import (
	"context"
	"fmt"
	"log/slog"

	"prototypus-ai-doc-go/internal/app"

	"github.com/shouni/go-remote-io/pkg/gcsfactory"
)

// buildRemoteIO は、GCS ベースの I/O コンポーネントを初期化します。
func buildRemoteIO(ctx context.Context) (rio *app.RemoteIO, err error) {
	factory, err := gcsfactory.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS factory: %w", err)
	}

	defer func() {
		if err != nil {
			if closeErr := factory.Close(); closeErr != nil {
				slog.Warn("failed to close GCS factory during cleanup", "error", closeErr)
			}
		}
	}()

	r, err := factory.InputReader()
	if err != nil {
		return nil, fmt.Errorf("failed to create input reader: %w", err)
	}
	w, err := factory.OutputWriter()
	if err != nil {
		return nil, fmt.Errorf("failed to create output writer: %w", err)
	}
	s, err := factory.URLSigner()
	if err != nil {
		return nil, fmt.Errorf("failed to create URL signer: %w", err)
	}
	return &app.RemoteIO{
		Factory: factory,
		Reader:  r,
		Writer:  w,
		Signer:  s,
	}, nil
}
