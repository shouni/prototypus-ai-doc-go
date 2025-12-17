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
	generatedScript, err := generate(ctx, opt)
	if err != nil {
		return err
	}
	if generatedScript == "" {
		return fmt.Errorf("AIによって生成されたスクリプトが空です")
	}
	err = publish(ctx, opt, generatedScript)
	if err != nil {
		return err
	}

	return nil
}

// generate は、すべての依存関係を構築し、スクリプトテキスト作成を実行します。
// 実行結果の文字列とエラーを返します。
func generate(
	ctx context.Context,
	opt config.GenerateOptions,
) (string, error) {

	runner, err := builder.BuildGenerateRunner(ctx, opt)
	if err != nil {
		// BuildReviewRunner が内部でアダプタやビルダーの構築エラーをラップして返す
		return "", fmt.Errorf("生成実行器の構築に失敗しました: %w", err)
	}
	generatedScript, err := runner.Run(ctx)
	if err != nil {
		return "", fmt.Errorf("スクリプトテキスト作成に失敗しました: %w", err)
	}

	return generatedScript, nil
}

// publish は、すべての依存関係を構築し、パブリッシュパイプラインを実行します。
func publish(
	ctx context.Context,
	opt config.GenerateOptions,
	scriptContent string,
) error {
	publishRunner, err := builder.BuildPublisherRunner(ctx, opt)
	if err != nil {
		return fmt.Errorf("PublishRunnerの構築に失敗しました: %w", err)
	}
	err = publishRunner.Run(ctx, scriptContent)
	if err != nil {
		return fmt.Errorf("公開処理の実行に失敗しました: %w", err)
	}

	return nil
}
