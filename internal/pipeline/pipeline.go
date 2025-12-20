package pipeline

import (
	"context"
	"fmt"
	"strings"

	"prototypus-ai-doc-go/internal/builder"
	"prototypus-ai-doc-go/internal/config"
)

// Execute は、すべての依存関係を構築し実行します。
func Execute(
	ctx context.Context,
	opts config.GenerateOptions,
) error {
	appCtx, err := builder.NewAppContext(ctx, opts)
	if err != nil {
		// AppContextの構築エラーをラップして返す
		return fmt.Errorf("AppContextの構築に失敗しました: %w", err)
	}
	if err := appCtx.Validate(); err != nil {
		return fmt.Errorf("AppContextの検証に失敗しました: %w", err)
	}
	generatedScript, err := generate(ctx, appCtx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(generatedScript) == "" {
		return fmt.Errorf("AIモデルが空のスクリプトを返しました。プロンプトや入力コンテンツに問題がないか確認してください")
	}
	err = publish(ctx, appCtx, generatedScript)
	if err != nil {
		return err
	}

	return nil
}

// generate は、すべての依存関係を構築し、スクリプトテキスト作成を実行します。
// 実行結果の文字列とエラーを返します。
func generate(
	ctx context.Context,
	appCtx builder.AppContext,
) (string, error) {
	generateRunner, err := builder.BuildGenerateRunner(ctx, appCtx)
	if err != nil {
		return "", fmt.Errorf("GenerateRunnerの構築に失敗しました: %w", err)
	}
	generatedScript, err := generateRunner.Run(ctx)
	if err != nil {
		return "", fmt.Errorf("スクリプトテキスト作成に失敗しました: %w", err)
	}

	return generatedScript, nil
}

// publish は、すべての依存関係を構築し、パブリッシュパイプラインを実行します。
func publish(
	ctx context.Context,
	appCtx builder.AppContext,
	scriptContent string,
) error {
	publishRunner, err := builder.BuildPublisherRunner(ctx, appCtx)
	if err != nil {
		return fmt.Errorf("PublisherRunnerの構築に失敗しました: %w", err)
	}
	err = publishRunner.Run(ctx, scriptContent)
	if err != nil {
		return fmt.Errorf("公開処理の実行に失敗しました: %w", err)
	}

	return nil
}
