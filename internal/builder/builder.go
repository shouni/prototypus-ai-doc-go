package builder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"prototypus-ai-doc-go/internal/prompt"
	"prototypus-ai-doc-go/internal/runner"

	"github.com/shouni/go-ai-client/v2/pkg/ai/gemini"
	"github.com/shouni/go-http-kit/pkg/httpkit"
	"github.com/shouni/go-remote-io/pkg/remoteio"
	"github.com/shouni/go-voicevox/pkg/voicevox"
	"github.com/shouni/go-web-exact/v2/pkg/extract"
)

// BuildGenerateRunner は、GenerateRunner のインスタンスを返します。
func BuildGenerateRunner(ctx context.Context, appCtx AppContext) (runner.GenerateRunner, error) {
	opts := appCtx.options
	extractor, err := extract.NewExtractor(appCtx.httpClient)
	if err != nil {
		return nil, fmt.Errorf("エクストラクタの初期化に失敗しました: %w", err)
	}
	reader, err := appCtx.remoteIOFactory.NewInputReader()
	if err != nil {
		return nil, fmt.Errorf("入力リーダの初期化に失敗しました: %w", err)
	}

	templateStr, err := prompt.GetPromptByMode(opts.Mode)
	if err != nil {
		return nil, err
	}
	promptBuilder, err := prompt.NewBuilder(templateStr)
	if err != nil {
		return nil, fmt.Errorf("プロンプトビルダーの作成に失敗しました: %w", err)
	}

	aiClient, err := initializeAIClient(ctx)
	if err != nil {
		return nil, err
	}

	return runner.NewDefaultGenerateRunner(
		opts,
		extractor,
		promptBuilder,
		aiClient,
		reader,
	), nil
}

// BuildPublisherRunner は、PublisherRunner のインスタンスを返します。
func BuildPublisherRunner(ctx context.Context, appCtx AppContext) (runner.PublisherRunner, error) {
	opts := appCtx.options
	writer, err := appCtx.remoteIOFactory.NewOutputWriter()
	if err != nil {
		return nil, fmt.Errorf("出力ライターの初期化に失敗しました: %w", err)
	}

	voicevoxExecutor, err := initializeVoicevoxExecutor(ctx, appCtx.httpClient, writer, opts.VoicevoxOutput)
	if err != nil {
		return nil, err
	}

	return runner.NewDefaultPublisherRunner(
		opts,
		voicevoxExecutor,
	), nil
}

// initializeAIClient は、gemini を初期化します。
func initializeAIClient(ctx context.Context) (*gemini.Client, error) {
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

// initializeVoicevoxExecutor は、VOICEVOX Executorを初期化します。
func initializeVoicevoxExecutor(ctx context.Context, httpClient httpkit.ClientInterface, writer remoteio.OutputWriter, voicevoxOutput string) (voicevox.EngineExecutor, error) {
	if voicevoxOutput == "" {
		slog.Info("VOICEVOXの出力先が未指定のため、エンジンエクゼキュータをスキップします。")
		return nil, nil
	}

	executor, err := voicevox.NewEngineExecutor(ctx, httpClient, writer, true)
	if err != nil {
		return nil, fmt.Errorf("VOICEVOXエンジンエクゼキュータの初期化に失敗しました: %w", err)
	}
	return executor, nil
}
