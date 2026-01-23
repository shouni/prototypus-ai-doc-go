package cmd

import (
	"fmt"
	"os"

	"github.com/shouni/clibase"
	"github.com/spf13/cobra"
)

// Execute は、アプリケーションのメインエントリポイントです。
func Execute() {
	clibase.Execute(clibase.App{
		Name:     "prototypus-ai-doc",
		AddFlags: addAppFlags,
		PreRunE:  preRunAppE,
		Commands: []*cobra.Command{
			generateCmd,
		},
	})
}

// addAppFlags は、アプリケーション固有の永続フラグを定義します。
// clibase により、標準で --verbose (-V) と --config (-C) が提供されています。
func addAppFlags(rootCmd *cobra.Command) {
	// 将来的に PAID Go 固有のグローバルフラグが必要になった場合は、ここに記述します。
}

// preRunAppE は、コマンド実行前に環境変数などの必須チェックを行います。
func preRunAppE(cmd *cobra.Command, args []string) error {
	// Gemini API を利用するため、APIキーの存在チェックは必須です。
	if os.Getenv("GEMINI_API_KEY") == "" {
		return fmt.Errorf("エラー: 環境変数 GEMINI_API_KEY が設定されていません。Gemini APIの利用には必須です")
	}

	return nil
}
