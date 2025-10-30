package cmd

import (
	"fmt"
	"os"

	// gcbase はフラグや共通構造体を利用するためにのみインポートを維持します。
	gcbase "github.com/shouni/go-cli-base"
	"github.com/spf13/cobra"
)

// グローバルなフラグ変数（--modelの値保持用）
var (
	model string
)

// ルートコマンドの基盤を作成するヘルパー関数
// gcbaseの Execute に依存せず、cobraを直接使用してルートコマンドを構築します。
func newRootCmd(appName string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   appName,
		Short: fmt.Sprintf("A CLI tool for %s.", appName),
		Long:  fmt.Sprintf("The CLI tool for %s. Use a subcommand to perform a task.", appName),

		// PersistentPreRunEを手動で実装 (clibaseの共通処理 + アプリ固有の処理)
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// clibase 共通の PersistentPreRun 処理
			if gcbase.Flags.Verbose {
				fmt.Println("Verbose mode enabled.")
			}

			// アプリケーション固有の PersistentPreRunE 処理
			return preRunAppE(cmd, args)
		},

		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	// 1. clibase 共通フラグを手動で追加
	rootCmd.PersistentFlags().BoolVarP(&gcbase.Flags.Verbose, "verbose", "V", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVarP(&gcbase.Flags.ConfigFile, "config", "c", "", "Config file path")

	// 2. アプリケーション固有のフラグを手動で追加
	addAppFlags(rootCmd)

	return rootCmd
}

// addAppFlags は、アプリケーション固有の永続フラグ（--model）を追加します。
func addAppFlags(rootCmd *cobra.Command) {
	defaultModel := "gemini-2.5-flash"
	rootCmd.PersistentFlags().StringVarP(&model, "model", "g", defaultModel, "使用する Google Gemini モデル名 (例: gemini-2.5-flash, gemini-2.5-pro)")
}

// preRunAppE は、アプリケーション固有の実行前チェック（GEMINI_API_KEY）を実行します。
func preRunAppE(cmd *cobra.Command, args []string) error {
	// GEMINI_API_KEY の必須チェック
	if os.Getenv("GEMINI_API_KEY") == "" {
		return fmt.Errorf("エラー: 環境変数 GEMINI_API_KEY が設定されていません。Gemini APIの利用には必須です")
	}
	return nil
}

// Execute は、アプリケーションのメインエントリポイントです。
func Execute() {
	appName := "prototypus-ai-doc"

	// ルートコマンドの構築
	rootCmd := newRootCmd(appName)

	// サブコマンドのフラグ定義と追加
	// initCmdFlags() は cmd/generate.go のフラグを初期化
	initCmdFlags()

	// generateCmd をルートに追加 (generateCmd は cmd/generate.go で初期化されている必要があります)
	rootCmd.AddCommand(generateCmd)

	// cobra の実行
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
