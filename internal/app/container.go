package app

import (
	"log/slog"

	"prototypus-ai-doc-go/internal/config"

	"github.com/shouni/go-http-kit/pkg/httpkit"
	"github.com/shouni/go-remote-io/pkg/remoteio"
)

// Container はアプリケーションの依存関係（DIコンテナ）を保持します。
type Container struct {
	Options *config.GenerateOptions
	// I/O and Storage
	RemoteIO *RemoteIO
	// External Adapters
	HTTPClient httpkit.ClientInterface
}

// RemoteIO は外部ストレージ操作に関するコンポーネントをまとめます。
type RemoteIO struct {
	Factory remoteio.IOFactory
	Reader  remoteio.InputReader
	Writer  remoteio.OutputWriter
	Signer  remoteio.URLSigner
}

// Close は、RemoteIO が保持する Factory などの内部リソースを解放します。
func (r *RemoteIO) Close() error {
	if r.Factory != nil {
		return r.Factory.Close()
	}
	return nil
}

// Close は、Container が保持するすべての外部接続リソースを安全に解放します。
func (c *Container) Close() {
	// RemoteIO のリソース解放を委譲
	if c.RemoteIO != nil {
		if err := c.RemoteIO.Close(); err != nil {
			slog.Error("failed to close RemoteIO", "error", err)
		}
	}
}
