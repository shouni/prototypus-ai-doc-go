package pipeline

import (
	"context"
	"fmt"

	"prototypus-ai-doc-go/internal/builder"
	"prototypus-ai-doc-go/internal/config"
)

// Execute は、すべての依存関係を構築し実行します。
func Execute(
	ctx context.Context,
	opt config.GenerateOptions,
) error {
	runner, err := builder.BuildGenerateRunner(ctx, opt)
	if err != nil {
		// BuildReviewRunner が内部でアダプタやビルダーの構築エラーをラップして返す
		return fmt.Errorf("生成実行器の構築に失敗しました: %w", err)
	}

	err = runner.Run(ctx)
	if err != nil {
		return err
	}

	return nil
}
