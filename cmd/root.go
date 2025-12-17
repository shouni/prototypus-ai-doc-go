package cmd

import (
	"fmt"
	"os"

	clibase "github.com/shouni/go-cli-base"
	"github.com/spf13/cobra"
)

// clibase.CustomFlagFunc のシグネチャに一致
func addAppFlags(rootCmd *cobra.Command) {
}

// preRunAppE は、アプリケーション固有の実行前チェック（GEMINI_API_KEY）を実行します。
// clibase.CustomPreRunEFunc のシグネチャに一致
func preRunAppE(cmd *cobra.Command, args []string) error {
	// GEMINI_API_KEY の必須チェック
	if os.Getenv("GEMINI_API_KEY") == "" {
		return fmt.Errorf("エラー: 環境変数 GEMINI_API_KEY が設定されていません。Gemini APIの利用には必須です")
	}

	return nil
}

// Execute は、アプリケーションのメインエントリポイントです。
func Execute() {
	clibase.Execute(
		"prototypus-ai-doc",
		addAppFlags,
		preRunAppE,
		generateCmd,
	)
}
