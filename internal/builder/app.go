package builder

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"prototypus-ai-doc-go/internal/app"
	"prototypus-ai-doc-go/internal/config"

	"github.com/shouni/go-http-kit/pkg/httpkit"
)

// BuildContainer は外部サービスとの接続を確立し、依存関係を組み立てた app.Container を返します。
func BuildContainer(ctx context.Context, options *config.GenerateOptions) (*app.Container, error) {
	var resources []io.Closer
	defer func() {
		for _, r := range resources {
			if r != nil {
				if closeErr := r.Close(); closeErr != nil {
					slog.Warn("failed to close resource during cleanup", "error", closeErr)
				}
			}
		}
	}()

	rio, err := buildRemoteIO(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize IO components: %w", err)
	}
	resources = append(resources, rio)

	timeout := options.HTTPTimeout
	if timeout == 0 {
		timeout = config.DefaultHTTPTimeout
	}

	httpClient := httpkit.New(
		timeout,
		httpkit.WithMaxRetries(1),
		httpkit.WithSkipNetworkValidation(true),
	)

	appCtx := &app.Container{
		Options:    options,
		RemoteIO:   rio,
		HTTPClient: httpClient,
	}

	return appCtx, nil
}
