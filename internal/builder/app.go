package builder

import (
	"context"
	"errors"
	"fmt"
	"prototypus-ai-doc-go/internal/config"

	"github.com/shouni/go-http-kit/pkg/httpkit"
	"github.com/shouni/go-remote-io/pkg/gcsfactory"
)

type AppContext struct {
	options         config.GenerateOptions
	httpClient      httpkit.ClientInterface
	remoteIOFactory gcsfactory.Factory
}

// NewAppContext は、依存関係の起点となる AppContext を生成します。
func NewAppContext(ctx context.Context, opts config.GenerateOptions) (AppContext, error) {
	timeout := opts.HTTPTimeout
	if timeout == 0 {
		timeout = config.DefaultHTTPTimeout
	}

	remoteIOFactory, err := gcsfactory.NewGCSClientFactory(ctx)
	if err != nil {
		return AppContext{}, fmt.Errorf("リモートストレージのクライアントファクトリ初期化に失敗しました: %w", err)
	}

	return AppContext{
		options:         opts,
		httpClient:      httpkit.New(timeout, httpkit.WithMaxRetries(3)),
		remoteIOFactory: remoteIOFactory,
	}, nil
}

// Close は、クライアント接続を安全にクローズします。
func (ac AppContext) Close() error {
	var multiErr error
	if ac.remoteIOFactory != nil {
		if err := ac.remoteIOFactory.Close(); err != nil {
			multiErr = errors.Join(multiErr, fmt.Errorf("GCS Factoryのクローズに失敗: %w", err))
		}
	}

	return multiErr
}

// Validate は、AppContextの必須フィールドがすべて正しく初期化されていることを検証します。
func (ac AppContext) Validate() error {
	if ac.httpClient == nil {
		return errors.New("HTTPClientが初期化されていません")
	}
	if ac.remoteIOFactory == nil {
		return errors.New("GCSFactoryが初期化されていません")
	}
	return nil
}
