package builder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"prototypus-ai-doc-go/internal/config"
	"prototypus-ai-doc-go/internal/prompt"
	"prototypus-ai-doc-go/internal/runner"

	"github.com/shouni/go-ai-client/v2/pkg/ai/gemini"
	"github.com/shouni/go-http-kit/pkg/httpkit"
	"github.com/shouni/go-remote-io/pkg/gcsfactory"
	"github.com/shouni/go-remote-io/pkg/remoteio"
	"github.com/shouni/go-voicevox/pkg/voicevox"
	"github.com/shouni/go-web-exact/v2/pkg/extract"
)

// initializeAIClient は、gemini を初期化します。
func initializeAIClient(ctx context.Context) (*gemini.Client, error) {
	// AI APIキーは環境変数からのみ取得
	finalAPIKey := os.Getenv("GEMINI_API_KEY")

	if finalAPIKey == "" {
		return nil, errors.New("AI APIキーが設定されていません。環境変数 GEMINI_API_KEY を確認してください。")
	}

	clientConfig := gemini.Config{
		APIKey: finalAPIKey,
	}

	aiClient, err := gemini.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("AIクライアントの初期化に失敗しました: %w", err)
	}
	return aiClient, nil
}

// initializeOutputWriter は、go-remote-io の OutputWriter を初期化します。
func initializeOutputWriter(ctx context.Context) (remoteio.OutputWriter, error) {
	gcsFactory, err := gcsfactory.NewGCSClientFactory(ctx)
	if err != nil {
		return nil, fmt.Errorf("リモートストレージのクライアントファクトリ初期化に失敗しました: %w", err)
	}
	writer, err := gcsFactory.NewOutputWriter()
	if err != nil {
		return nil, fmt.Errorf("出力ライターの初期化に失敗しました: %w", err)
	}

	return writer, nil
}

// initializeVoicevoxExecutor は、VOICEVOX Executorを初期化し、不要な場合は nil を返します。
func initializeVoicevoxExecutor(ctx context.Context, httpClient httpkit.ClientInterface, writer remoteio.OutputWriter, voicevoxOutput string) (voicevox.EngineExecutor, error) {
	if voicevoxOutput == "" {
		slog.Info("VOICEVOXの出力先が未指定のため、エンジンエクゼキュータをスキップします。")
		return nil, nil // Executorインターフェースに対して nil を返す
	}

	executor, err := voicevox.NewEngineExecutor(ctx, httpClient, writer, true)
	if err != nil {
		return nil, fmt.Errorf("VOICEVOXエンジンエクゼキュータの初期化に失敗しました: %w", err)
	}
	return executor, nil
}

// BuildGenerateRunner は、必要な依存関係をすべて構築し、
// 実行可能な GenerateRunner のインスタンスを返します。
func BuildGenerateRunner(ctx context.Context, appContext config.AppContext) (runner.GenerateRunner, error) {
	if appContext.HTTPClient == nil {
		return nil, errors.New("AppContext 内の HTTPClient が初期化されていません。")
	}

	opts := appContext.Options
	extractor, err := extract.NewExtractor(appContext.HTTPClient)
	if err != nil {
		return nil, fmt.Errorf("エクストラクタの初期化に失敗しました: %w", err)
	}

	// promptBuilderの初期化
	templateStr, err := prompt.GetPromptByMode(opts.Mode)
	if err != nil {
		return nil, err // モードが無効な場合のエラー
	}
	promptBuilder, err := prompt.NewBuilder(templateStr)
	if err != nil {
		// NewBuilderが解析エラーを返した場合は、それをラップして返却
		return nil, fmt.Errorf("プロンプトビルダーの作成に失敗しました: %w", err)
	}

	// AIクライアントの初期化
	aiClient, err := initializeAIClient(ctx)
	if err != nil {
		return nil, err
	}

	generateRunner := runner.NewDefaultGenerateRunner(
		opts,
		extractor,
		promptBuilder,
		aiClient,
	)

	return generateRunner, nil
}

// BuildPublisherRunner は、必要な依存関係をすべて構築し、
// 実行可能な PublisherRunner のインスタンスを返します。
func BuildPublisherRunner(ctx context.Context, appContext config.AppContext) (runner.PublisherRunner, error) {
	if appContext.HTTPClient == nil {
		return nil, errors.New("AppContext 内の HTTPClient が初期化されていません。")
	}

	opts := appContext.Options
	writer, err := initializeOutputWriter(ctx)
	if err != nil {
		return nil, err
	}

	// VOICEVOX エンジンパイプラインの初期化
	voicevoxExecutor, err := initializeVoicevoxExecutor(ctx, appContext.HTTPClient, writer, opts.VoicevoxOutput)
	if err != nil {
		return nil, err
	}

	publisherRunner := runner.NewDefaultPublisherRunner(
		opts,
		voicevoxExecutor,
	)

	return publisherRunner, nil
}
