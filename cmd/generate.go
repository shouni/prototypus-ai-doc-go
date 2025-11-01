package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"prototypus-ai-doc-go/internal/pipeline"

	"github.com/shouni/go-ai-client/v2/pkg/ai/gemini"
	"github.com/shouni/go-http-kit/pkg/httpkit"
	"github.com/shouni/go-voicevox/pkg/voicevox"
	"github.com/shouni/go-web-exact/v2/pkg/extract"
	"github.com/spf13/cobra"
)

// グローバルなオプションインスタンス。
var opts pipeline.GenerateOptions

// defaultVoicevoxAPIURL は、VOICEVOX APIのデフォルトURLです。
const defaultVoicevoxAPIURL = "http://localhost:50021"

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
	// AI APIキーは環境変数からのみ取得 (clibaseの PersistentPreRunE で GEMINI_API_KEY の存在は保証済みと想定)
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
	// opts.HTTPTimeoutがゼロ値の場合、デフォルト値を使用
	httpTimeout := opts.HTTPTimeout
	if httpTimeout == 0 {
		httpTimeout = 30 * time.Second
	}

	// 1. 共通依存関係の初期化 (HTTPクライアント/Extractor)
	fetcher := httpkit.New(httpTimeout, httpkit.WithMaxRetries(3))
	extractor, err := extract.NewExtractor(fetcher)
	if err != nil {
		return pipeline.GenerateHandler{}, fmt.Errorf("エクストラクタの初期化に失敗しました: %w", err)
	}

	// 2. Gemini Clientの初期化
	aiClient, err := initializeAIClient(ctx)
	if err != nil {
		return pipeline.GenerateHandler{}, err
	}

	// 3. VOICEVOX Clientの初期化
	var voicevoxClient *voicevox.Client
	if opts.VoicevoxOutput != "" {
		voicevoxAPIURL := os.Getenv("VOICEVOX_API_URL")
		if voicevoxAPIURL == "" {
			voicevoxAPIURL = defaultVoicevoxAPIURL
			fmt.Fprintf(os.Stderr, "警告: VOICEVOX_API_URL 環境変数が設定されていません。デフォルト値 (%s) を使用します。\n", voicevoxAPIURL)
		}
		voicevoxClient = voicevox.NewClient(voicevoxAPIURL, httpTimeout)
	}

	// 3. Handlerに依存関係を注入
	handler := pipeline.GenerateHandler{
		Options:        opts,
		Extractor:      extractor,
		AiClient:       aiClient,
		VoicevoxClient: voicevoxClient,
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
