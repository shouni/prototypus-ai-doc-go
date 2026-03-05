package domain

import (
	"context"
)

// Pipeline は、デコードされたペイロードを受け取って実際の処理を行うインターフェースです。
type Pipeline interface {
	// Execute は、指定されたコンテキストに基づいて GenerateTaskPayload を処理し、問題が発生した場合はエラーを返します。
	Execute(ctx context.Context) error
}

// GenerateRunner は、生成されたコンテンツまたはエラーに関する通知を指定されたターゲットまたはチャネルに送信するためのインターフェイスです。
type GenerateRunner interface {
	Run(ctx context.Context) (string, error)
}

// PublishRunner は、生成されたコンテンツまたはエラーに関する通知を指定されたターゲットまたはチャネルに送信するためのインターフェイスです。
type PublishRunner interface {
	Run(ctx context.Context, scriptContent string) error
}
