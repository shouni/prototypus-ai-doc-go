package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"prototypus-ai-doc-go/internal/pipeline"
	"prototypus-ai-doc-go/internal/prompt"

	"github.com/shouni/go-ai-client/v2/pkg/ai/gemini"
	"github.com/shouni/go-http-kit/pkg/httpkit"
	"github.com/shouni/go-voicevox/pkg/voicevox"
	"github.com/shouni/go-web-exact/v2/pkg/extract"
	"github.com/spf13/cobra"
)

// グローバルなオプションインスタンス。
var opts pipeline.GenerateOptions

// defaultHTTPTimeout はHTTPリクエストのデフォルトタイムアウトを定義します。
// defaultVoicevoxAPIURL はVOICEVOX APIのデフォルトURLです。
// NOTE: voicevox.DefaultMaxParallelSegments, voicevox.DefaultSegmentTimeout はvoicevoxパッケージから直接参照
const (
	defaultHTTPTimeout = 30 * time.Second
)

// defaultModel specifies the default Google Gemini model name used when no model is explicitly provided.
const defaultModel = "gemini-2.5-flash"

// generateCmd はナレーションスクリプト生成のメインコマンドです。
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "AIにナレーションスクリプトを生成させます。",
	Long: `AIに渡す元となる文章を指定し、ナレーションスクリプトを生成します。
Webページやファイル、標準入力から文章を読み込むことができます。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		// --- 1. 依存関係をセットアップし、Handlerを取得 ---
		handler, err := setupDependencies(ctx)
		if err != nil {
			return err // 初期化失敗
		}

		// --- 2. 実行ロジック ---
		return handler.RunGenerate(ctx)
	},
}

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

// setupDependencies は、RunEの実行に必要な全ての依存関係（クライアント、エクストラクタなど）を初期化し、
// RunGenerateを実行するためのHandlerを返します。
func setupDependencies(ctx context.Context) (pipeline.GenerateHandler, error) {
	// --- タイムアウト値の調整 ---
	httpTimeout := opts.HTTPTimeout
	if httpTimeout == 0 {
		httpTimeout = defaultHTTPTimeout
	}

	// 1. 共通依存関係の初期化 (HTTPクライアント/Extractor)
	fetcher := httpkit.New(httpTimeout, httpkit.WithMaxRetries(3))
	extractor, err := extract.NewExtractor(fetcher)
	if err != nil {
		return pipeline.GenerateHandler{}, fmt.Errorf("エクストラクタの初期化に失敗しました: %w", err)
	}

	// 2. promptBuilderの初期化
	templateStr, err := prompt.GetPromptByMode(opts.Mode)
	if err != nil {
		return pipeline.GenerateHandler{}, err // モードが無効な場合のエラー
	}
	promptBuilder, err := prompt.NewBuilder(templateStr)
	if err != nil {
		// NewBuilderが解析エラーを返した場合は、それをラップして返却
		return pipeline.GenerateHandler{}, fmt.Errorf("プロンプトビルダーの作成に失敗しました: %w", err)
	}

	// 3. AIクライアントの初期化
	aiClient, err := initializeAIClient(ctx)
	if err != nil {
		return pipeline.GenerateHandler{}, err
	}

	// 4. VOICEVOX エンジンパイプラインの初期化
	voicevoxExecutor, err := voicevox.NewEngineExecutor(ctx, httpTimeout, opts.VoicevoxOutput != "")
	if err != nil {
		return pipeline.GenerateHandler{}, err
	}

	// 5. Handlerに依存関係を注入
	// pipeline.GenerateHandler のフィールドがインターフェース型であっても、
	// ここで渡しているのは具象型(*prompt.Builder, *voicevox.Engine)なので問題なく代入される
	handler := pipeline.GenerateHandler{
		Options:                opts,
		Extractor:              extractor,
		PromptBuilder:          promptBuilder,
		AiClient:               aiClient,
		VoicevoxEngineExecutor: voicevoxExecutor,
	}

	return handler, nil
}

// initCmdFlags は generateCmd のフラグ定義を行います。
func initCmdFlags() {
	generateCmd.Flags().StringVarP(&opts.ScriptURL,
		"script-url", "u", "", "Webページからコンテンツを取得するためのURL。")
	generateCmd.Flags().StringVarP(&opts.ScriptFile,
		"script-file", "f", "", "入力スクリプトファイルのパス ('-'を指定すると標準入力から読み込みます。)")
	generateCmd.Flags().StringVarP(&opts.OutputFile,
		"output-file", "o", "", "生成されたスクリプトを保存するファイルのパス。省略時は標準出力 (stdout) に出力します。")
	generateCmd.Flags().StringVarP(&opts.Mode,
		"mode", "m", "duet", "スクリプト生成モード。'dialogue', 'solo', 'duet' などを指定します。")
	generateCmd.Flags().BoolVarP(&opts.PostAPI,
		"post-api", "p", false, "生成されたスクリプトを外部APIに投稿します。")
	generateCmd.Flags().StringVarP(&opts.VoicevoxOutput,
		"voicevox", "v", "", "生成されたスクリプトをVOICEVOXエンジンで合成し、指定されたファイル名に出力します (例: output.wav)。")
	generateCmd.Flags().DurationVar(&opts.HTTPTimeout,
		"http-timeout", 30*time.Second, "Webリクエストのタイムアウト時間 (例: 15s, 1m)。")
	generateCmd.Flags().StringVarP(&opts.AIModel,
		"model", "g", defaultModel, "使用する Google Gemini モデル名 (例: gemini-2.5-flash, gemini-2.5-pro)")
}
