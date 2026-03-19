package app

import (
	"errors"

	"github.com/shouni/go-http-kit/pkg/httpkit"
	"github.com/shouni/go-remote-io/pkg/remoteio"

	"prototypus-ai-doc-go/internal/config"
	"prototypus-ai-doc-go/internal/domain"
)

// Container はアプリケーションの依存関係（DIコンテナ）を保持します。
type Container struct {
	Config *config.Config
	// I/O and Storage
	RemoteIO *RemoteIO
	// External Adapters
	HTTPClient httpkit.RequestExecutor
	// Business Logic
	Pipeline domain.Pipeline
}

// RemoteIO は外部ストレージ操作に関するコンポーネントをまとめます。
type RemoteIO struct {
	Factory remoteio.IOFactory
	Reader  remoteio.InputReader
	Writer  remoteio.OutputWriter
}

// Close は、RemoteIO が保持する Factory などの内部リソースを解放します。
func (r *RemoteIO) Close() error {
	if r == nil {
		return nil
	}
	if r.Factory != nil {
		return r.Factory.Close()
	}
	return nil
}

// Close は、Container が保持するすべての外部接続リソースを安全に解放します。
func (c *Container) Close() error {
	if c == nil {
		return nil
	}
	var errs error
	if c.RemoteIO != nil {
		if err := c.RemoteIO.Close(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}
