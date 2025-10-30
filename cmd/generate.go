package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/shouni/go-web-exact/v2/pkg/client"
	"github.com/shouni/go-web-exact/v2/pkg/extract"
	"github.com/spf13/cobra"
	"prototypus-ai-doc-go/internal/generator"
	"prototypus-ai-doc-go/internal/voicevox"
)

// グローバルなオプションインスタンス。
var opts generator.GenerateOptions

// defaultVoicevoxAPIURL は、generatorパッケージからインポートできないため、ここで再定義するか、直接使用する。
// generatorパッケージ内にも定数として定義されているが、ここではVOICEVOX Clientの初期化にのみ必要。
const defaultVoicevoxAPIURL = "http://localhost:50021"

// generateCmd はナレーションスクリプト生成のメインコマンドです。
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "AIにナレーションスクリプトを生成させます。",
	Long: `AIに渡す元となる文章を指定し、ナレーションスクリプトを生成します。
Webページやファイル、標準入力から文章を読み込むことができます。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		// --- タイムアウト値の調整 ---
		// フラグが指定されなかった場合、opts.HTTPTimeoutはDurationのゼロ値(0)になる。
		// その場合は、init()で設定したデフォルト値(30s)を明示的に使用する。
		httpTimeout := opts.HTTPTimeout
		if httpTimeout == 0 {
			httpTimeout = 30 * time.Second
		}

		// 共通依存関係の初期化 (HTTPクライアント/Extractor)
		fetcher := client.New(httpTimeout, client.WithMaxRetries(5))

		// NewExtractorがエラーを返すため、チェックを追加
		extractor, err := extract.NewExtractor(fetcher)
		if err != nil {
			return fmt.Errorf("エクストラクタの初期化に失敗しました: %w", err)
		}

		// VOICEVOX Clientの初期化（DIの徹底）
		var voicevoxClient *voicevox.Client
		if opts.VoicevoxOutput != "" {
			voicevoxAPIURL := os.Getenv("VOICEVOX_API_URL")
			if voicevoxAPIURL == "" {
				voicevoxAPIURL = defaultVoicevoxAPIURL
				fmt.Fprintf(os.Stderr, "警告: VOICEVOX_API_URL 環境変数が設定されていません。デフォルト値 (%s) を使用します。\n", voicevoxAPIURL)
			}
			// fetcher を Voicevox Client の Doer として渡す
			voicevoxClient = voicevox.NewClient(voicevoxAPIURL, fetcher)
		}

		// generatorパッケージのGenerateHandlerを使用し、全ての依存関係を注入
		handler := generator.GenerateHandler{
			Options:        opts,
			Extractor:      extractor,
			VoicevoxClient: voicevoxClient,
		}

		// RunGenerate メソッドを呼び出し
		return handler.RunGenerate(ctx)
	},
}

// --------------------------------------------------------------------------------
// init() と フラグ定義
// --------------------------------------------------------------------------------

func init() {
	rootCmd.AddCommand(generateCmd)

	// --- フラグ定義 ---
	generateCmd.Flags().StringVarP(&opts.ScriptURL, "script-url", "u", "", "Webページからコンテンツを取得するためのURL。")
	generateCmd.Flags().StringVarP(&opts.ScriptFile, "script-file", "f", "", "入力スクリプトファイルのパス ('-'を指定すると標準入力から読み込みます。)")
	generateCmd.Flags().StringVarP(&opts.OutputFile, "output-file", "o", "",
		"生成されたスクリプトを保存するファイルのパス。省略時は標準出力 (stdout) に出力します。")
	generateCmd.Flags().StringVarP(&opts.Mode, "mode", "m", "duet",
		"スクリプト生成モード。'dialogue', 'solo', 'duet' などを指定します。")
	generateCmd.Flags().BoolVarP(&opts.PostAPI, "post-api", "p", false,
		"生成されたスクリプトを外部APIに投稿します。")
	generateCmd.Flags().StringVarP(&opts.VoicevoxOutput, "voicevox", "v", "",
		"生成されたスクリプトをVOICEVOXエンジンで合成し、指定されたファイル名に出力します (例: output.wav)。")
	generateCmd.Flags().DurationVar(&opts.HTTPTimeout, "http-timeout", 30*time.Second,
		"Webリクエストのタイムアウト時間 (例: 15s, 1m)。")

	// AI クライアント設定フラグ
	generateCmd.Flags().StringVar(&opts.AIAPIKey, "ai-api-key", "",
		"Google Gemini APIキー。環境変数 GEMINI_API_KEY を上書きします。")
	generateCmd.Flags().StringVar(&opts.AIModel, "ai-model", "gemini-2.5-flash",
		"使用するGeminiモデル名。")
}
