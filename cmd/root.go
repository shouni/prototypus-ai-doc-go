package cmd

import (
	"fmt"
	"os"

	clibase "github.com/shouni/go-cli-base"
	"github.com/spf13/cobra"
)

// グローバルなフラグ変数（--modelの値保持用）
// これはclibase.Flagsのように、外部からアクセスできるよう維持します。
var (
	Model string // 大文字に変更することで、他のパッケージからのアクセスを可能にする
)

// addAppFlags は、アプリケーション固有の永続フラグ（--model）を追加します。
// clibase.CustomFlagFunc のシグネチャに一致
func addAppFlags(rootCmd *cobra.Command) {
	defaultModel := "gemini-2.5-flash"
	// clibase が提供する rootCmd に対してフラグを追加する
	rootCmd.PersistentFlags().StringVarP(&Model, "model", "g", defaultModel, "使用する Google Gemini モデル名 (例: gemini-2.5-flash, gemini-2.5-pro)")
}

// preRunAppE は、アプリケーション固有の実行前チェック（GEMINI_API_KEY）を実行します。
// clibase.CustomPreRunEFunc のシグネチャに一致
func preRunAppE(cmd *cobra.Command, args []string) error {
	// GEMINI_API_KEY の必須チェック
	if os.Getenv("GEMINI_API_KEY") == "" {
		return fmt.Errorf("エラー: 環境変数 GEMINI_API_KEY が設定されていません。Gemini APIの利用には必須です")
	}
	// clibase 共通フラグ (Verbose/Config) は clibase 側で既に処理されている
	return nil
}

// Execute は、アプリケーションのメインエントリポイントです。
func Execute() {
	appName := "prototypus-ai-doc"

	// サブコマンドのフラグ定義と初期化 (generateCmdの初期化が必要)
	// initCmdFlags() は引き続き必要
	initCmdFlags()

	// ルートコマンドの構築と実行を clibase に全て委任
	// 共通フラグ（--verbose, --config）が自動で追加され、
	// PersistentPreRunEに clibaseの共通処理 と preRunAppE が結合される。
	clibase.Execute(
		appName,
		addAppFlags, // 固有フラグの追加ロジック
		preRunAppE,  // 固有の実行前チェックロジック
		generateCmd, // 追加するサブコマンド
	)
}
