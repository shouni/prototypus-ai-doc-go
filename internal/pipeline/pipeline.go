package pipeline

import (
	"context"
	"fmt"
	"strings"

	"prototypus-ai-doc-go/internal/domain"
)

// Pipeline はパイプラインの実行に必要な外部依存関係を保持するサービス構造体です。
type Pipeline struct {
	generateRunner domain.GenerateRunner
	publishRunner  domain.PublishRunner
}

// NewPipeline は、Container から必要な依存関係のみを抽出して MangaPipeline を生成します。
func NewPipeline(generateRunner domain.GenerateRunner, publishRunner domain.PublishRunner) *Pipeline {
	return &Pipeline{
		generateRunner: generateRunner,
		publishRunner:  publishRunner,
	}
}

// Execute は、すべての依存関係を構築し実行します。
func (p *Pipeline) Execute(
	ctx context.Context,
) error {
	generatedScript, err := p.generate(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(generatedScript) == "" {
		return fmt.Errorf("AIモデルが空のスクリプトを返しました。プロンプトや入力コンテンツに問題がないか確認してください")
	}
	err = p.publish(ctx, generatedScript)
	if err != nil {
		return err
	}

	return nil
}

// generate は、すべての依存関係を構築し、スクリプトテキスト作成を実行します。
// 実行結果の文字列とエラーを返します。
func (p *Pipeline) generate(
	ctx context.Context,
) (string, error) {
	generatedScript, err := p.generateRunner.Run(ctx)
	if err != nil {
		return "", fmt.Errorf("スクリプトテキスト作成に失敗しました: %w", err)
	}

	return generatedScript, nil
}

// publish は、すべての依存関係を構築し、パブリッシュパイプラインを実行します。
func (p *Pipeline) publish(
	ctx context.Context,
	scriptContent string,
) error {
	err := p.publishRunner.Run(ctx, scriptContent)
	if err != nil {
		return fmt.Errorf("公開処理の実行に失敗しました: %w", err)
	}

	return nil
}
