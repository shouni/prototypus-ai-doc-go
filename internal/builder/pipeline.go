package builder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"prototypus-ai-doc-go/internal/app"
	"prototypus-ai-doc-go/internal/domain"
	"prototypus-ai-doc-go/internal/pipeline"
	"prototypus-ai-doc-go/internal/prompt"
	"prototypus-ai-doc-go/internal/runner"

	"github.com/shouni/go-gemini-client/pkg/gemini"
	"github.com/shouni/go-http-kit/pkg/httpkit"
	"github.com/shouni/go-remote-io/pkg/remoteio"
	"github.com/shouni/go-voicevox/pkg/voicevox"
	"github.com/shouni/go-web-exact/v2/pkg/extract"
)

// buildPipeline は、提供されたランナーを使用して新しいパイプラインを初期化して返します。
func buildPipeline(ctx context.Context, appCtx *app.Container) (domain.Pipeline, error) {
	generateRunner, err := buildGenerateRunner(ctx, appCtx)
	if err != nil {
		return nil, fmt.Errorf("生成ランナーの初期化に失敗しました: %w", err)
	}
	publisherRunner, err := buildPublishRunner(ctx, appCtx)
	if err != nil {
		return nil, fmt.Errorf("パブリッシャーランナーの初期化に失敗しました: %w", err)
	}

	p := pipeline.NewPipeline(generateRunner, publisherRunner)

	return p, nil
}

// buildGenerateRunner は、GenerateRunner のインスタンスを返します。
func buildGenerateRunner(ctx context.Context, appCtx *app.Container) (domain.GenerateRunner, error) {
	opts := appCtx.Options
	extractor, err := extract.NewExtractor(appCtx.HTTPClient)
	if err != nil {
		return nil, fmt.Errorf("エクストラクタの初期化に失敗しました: %w", err)
	}

	promptBuilder, err := prompt.NewBuilder()
	if err != nil {
		return nil, fmt.Errorf("プロンプトビルダーの作成に失敗しました: %w", err)
	}

	aiClient, err := initializeAIClient(ctx)
	if err != nil {
		return nil, err
	}

	return runner.NewGenerateRunner(
		opts,
		extractor,
		promptBuilder,
		aiClient,
		appCtx.RemoteIO.Reader,
	), nil
}

// buildPublishRunner は、PublisherRunner のインスタンスを返します。
func buildPublishRunner(ctx context.Context, appCtx *app.Container) (domain.PublishRunner, error) {
	opts := appCtx.Options
	voicevoxExecutor, err := initializeVoicevoxExecutor(ctx, appCtx.HTTPClient, appCtx.RemoteIO.Writer, opts.VoicevoxOutput)
	if err != nil {
		return nil, err
	}

	return runner.NewPublisherRunner(
		opts,
		voicevoxExecutor,
		appCtx.RemoteIO.Writer,
	), nil
}

// initializeAIClient は、GEMINI_API_KEY が存在すればそれを優先し、
// なければ Vertex AI (PROJECT_ID) を使用してクライアントを初期化します。
func initializeAIClient(ctx context.Context) (gemini.Generator, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")

	if apiKey != "" {
		return gemini.NewClient(ctx, gemini.Config{
			APIKey: apiKey,
		})
	}

	projectID := os.Getenv("PROJECT_ID")
	if projectID == "" {
		return nil, errors.New("GEMINI_API_KEY または PROJECT_ID が必要です。環境変数を確認してください。")
	}

	return gemini.NewClient(ctx, gemini.Config{
		ProjectID:  projectID,
		LocationID: "global",
	})
}

// initializeVoicevoxExecutor は、VOICEVOX Executorを初期化します。
func initializeVoicevoxExecutor(ctx context.Context, httpClient httpkit.RequestExecutor, writer remoteio.OutputWriter, voicevoxOutput string) (voicevox.EngineExecutor, error) {
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
