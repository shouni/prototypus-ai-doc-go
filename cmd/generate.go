package cmd

import (
	"fmt"
	"prototypus-ai-doc-go/internal/config"
	"prototypus-ai-doc-go/internal/pipeline"

	"github.com/spf13/cobra"
)

// グローバルなオプションインスタンス。
var opts config.GenerateOptions

// generateCmd はナレーションスクリプト生成のメインコマンドです。
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "AIにナレーションスクリプトを生成させます。",
	Long: `AIに渡す元となる文章を指定し、ナレーションスクリプトを生成します。
Webページやファイル、標準入力から文章を読み込むことができます。`,
	RunE: generateCommand,
}

// init は generateCommand のフラグ定義を行います。
func init() {
	generateCmd.Flags().StringVarP(&opts.ScriptURL, "script-url", "u", "", "Webページからコンテンツを取得するためのURL。")
	generateCmd.Flags().StringVarP(&opts.ScriptFile, "script-file", "f", "", "入力スクリプトファイルのパス ('-'を指定すると標準入力から読み込みます。)")
	generateCmd.Flags().StringVarP(&opts.OutputFile, "output-file", "o", "", "生成されたスクリプトを保存するファイルのパス。省略時は標準出力 (stdout) に出力します。")
	generateCmd.Flags().StringVarP(&opts.Mode, "mode", "m", "duet", "スクリプト生成モード。'dialogue', 'solo', 'duet' などを指定します。")
	generateCmd.Flags().StringVarP(&opts.VoicevoxOutput, "voicevox", "v", "", "生成されたスクリプトをVOICEVOXエンジンで合成し、指定されたパスに出力します (例: output.wav, gs://my-bucket/audio.wav)。")
	generateCmd.Flags().StringVarP(&opts.AIModel, "model", "g", config.DefaultModel, "使用する Google Gemini モデル名 (例: gemini-2.5-flash, gemini-2.5-pro)")
	generateCmd.Flags().DurationVar(&opts.HTTPTimeout, "http-timeout", config.DefaultHTTPTimeout, "Webリクエストのタイムアウト時間 (例: 15s, 1m)。")
}

// generateCommand は、AIによるナレーションスクリプトを生成し、指定されたURIのクラウドストレージにWAVをアップロード
func generateCommand(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// 制約チェック
	if cmd.Flags().Changed("voicevox") && cmd.Flags().Changed("output-file") {
		return fmt.Errorf("voicevox出力(-v)とファイル出力(-o)は同時に指定できません。どちらか一方のみ指定してください")
	}

	err := pipeline.Execute(ctx, opts)
	if err != nil {
		return err
	}

	return nil
}
