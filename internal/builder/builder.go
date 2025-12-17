package builder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"prototypus-ai-doc-go/internal/config"
	"prototypus-ai-doc-go/internal/prompt"
	"prototypus-ai-doc-go/internal/runner"

	"github.com/shouni/go-ai-client/v2/pkg/ai/gemini"
	"github.com/shouni/go-http-kit/pkg/httpkit"
	"github.com/shouni/go-remote-io/pkg/gcsfactory"
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

// initializeGCSFactory は、go-remote-io の GCS Factory を初期化します。
func initializeGCSFactory(ctx context.Context) (gcsfactory.Factory, error) {
	gcsFactory, err := gcsfactory.NewGCSClientFactory(ctx)
	if err != nil {
		return nil, fmt.Errorf("GCSファクトリの初期化に失敗しました: %w", err)
	}

	return gcsFactory, nil
}

// initializeVoicevoxExecutor は、VOICEVOX Executorを初期化し、不要な場合は nil を返します。
func initializeVoicevoxExecutor(ctx context.Context, voicevoxOutput string, httpTimeout time.Duration, gcsFactory gcsfactory.Factory) (voicevox.EngineExecutor, error) {
	if voicevoxOutput == "" {
		slog.Info("VOICEVOXの出力先が未指定のため、エンジンエクゼキュータをスキップします。")
		return nil, nil // Executorインターフェースに対して nil を返す
	}

	executor, err := voicevox.NewEngineExecutor(ctx, httpTimeout, true, gcsFactory)
	if err != nil {
		return nil, fmt.Errorf("VOICEVOXエンジンエクゼキュータの初期化に失敗しました: %w", err)
	}
	return executor, nil
}

// BuildGenerateRunner は、必要な依存関係をすべて構築し、
// 実行可能な GenerateRunner のインスタンスを返します。
func BuildGenerateRunner(ctx context.Context, opts config.GenerateOptions) (runner.GenerateRunner, error) {
	// --- タイムアウト値の調整 ---
	httpTimeout := opts.HTTPTimeout
	if httpTimeout == 0 {
		httpTimeout = config.DefaultHTTPTimeout
	}

	// 共通依存関係の初期化 (HTTPクライアント/Extractor)
	fetcher := httpkit.New(httpTimeout, httpkit.WithMaxRetries(3))
	extractor, err := extract.NewExtractor(fetcher)
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

	// GCS Factoryの初期化
	gcsFactory, err := initializeGCSFactory(ctx)
	if err != nil {
		return nil, err
	}

	// VOICEVOX エンジンパイプラインの初期化
	voicevoxExecutor, err := initializeVoicevoxExecutor(ctx, opts.VoicevoxOutput, httpTimeout, gcsFactory)
	if err != nil {
		return nil, err
	}

	// Handlerに依存関係を注入
	generateRunner := runner.NewDefaultGenerateRunner(
		opts,
		extractor,
		promptBuilder,
		aiClient,
		voicevoxExecutor,
	)

	return generateRunner, nil
}
