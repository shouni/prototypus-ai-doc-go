package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// グローバルなフラグ変数を定義
var (
	// AIモデル名 (例: gemini-2.5-flash)
	model string
)

// rootCmd は、引数なしで実行されたときの基点となるコマンドです。
var rootCmd = &cobra.Command{
	Use:   "prototypus-ai-doc",
	Short: "文章をずんだもん＆めたんの対話スクリプトに変換するAI CLIツール",
	Long: `
prototypus-ai-doc は、Google Gemini API を利用して、
与えられた文章を、話者や感情タグ付きのナレーションスクリプトに変換します。
`,
	// 全てのコマンド実行前に呼ばれる関数です。
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// 1. GEMINI_API_KEY の必須チェック
		if os.Getenv("GEMINI_API_KEY") == "" {
			return fmt.Errorf("エラー: 環境変数 GEMINI_API_KEY が設定されていません。Gemini APIの利用には必須です")
		}

		// 2. モデル名のバリデーション（必要に応じてここにチェックを追加できます）

		return nil
	},
	// サブコマンドを持たない場合の実行ロジック（今回はサブコマンドを必須とするため空）
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// Execute は、rootCmd を実行するメイン関数です。
// main.go から呼び出されます。
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// ここで永続フラグ（すべてのサブコマンドで利用可能）を定義します。

	// --model フラグの定義
	rootCmd.PersistentFlags().StringVarP(&model, "model", "", "gemini-2.5-flash",
		"使用する Google Gemini モデル名 (例: gemini-2.5-flash, gemini-2.5-pro)")

	// generate コマンドをルートに追加（generate.goで定義します）
	// TODO: rootCmd.AddCommand(generateCmd) を generate.go で定義後に有効化
}

// -----------------------------------------------------------------
// この下の generateCmd は、次のステップ cmd/generate.go で作成します
// -----------------------------------------------------------------
