package cmd

import (
	"time"

	"github.com/shouni/go-web-exact/pkg/httpclient"
	"github.com/spf13/cobra"

	webextractor "github.com/shouni/go-web-exact/pkg/web"

	"prototypus-ai-doc-go/internal/generator"
)

// グローバルなオプションインスタンス。
var opts generator.GenerateOptions

// generateCmd はナレーションスクリプト生成のメインコマンドです。
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "AIにナレーションスクリプトを生成させます。",
	Long: `AIに渡す元となる文章を指定し、ナレーションスクリプトを生成します。
Webページやファイル、標準入力から文章を読み込むことができます。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		// 依存関係の初期化
		fetcher := httpclient.New(opts.HTTPTimeout)
		extractor := webextractor.NewExtractor(fetcher)

		// generatorパッケージのGenerateHandlerを使用し、依存関係を注入
		handler := generator.GenerateHandler{
			Options:   opts,
			Extractor: extractor,
		}

		// RunGenerate メソッドを呼び出し（メソッドがgeneratorパッケージに移動したため）
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
	generateCmd.Flags().StringVarP(&opts.ScriptFile, "script-file", "f", "", "入力スクリプトファイルのパス ('-'を指定すると標準入力から読み込みます)。")
	generateCmd.Flags().StringVarP(&opts.OutputFile, "output-file", "o", "",
		"生成されたスクリプトを保存するファイルのパス。省略時は標準出力 (stdout) に出力します。")
	generateCmd.Flags().StringVarP(&opts.Mode, "mode", "m", "solo",
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
	generateCmd.Flags().StringVar(&opts.AIURL, "ai-url", "",
		"Gemini APIのベースURL。現在のライブラリ構造では、このフラグによるAPIエンドポイントのカスタマイズはサポートされていません。")
}
