package cmd

import (
	"fmt"
	"os"

	// 正しいインポートパス: github.com/shouni/go-cli-base
	gcbase "github.com/shouni/go-cli-base"
	"github.com/spf13/cobra"
)

// グローバルなフラグ変数
var (
	model string
)

// ルートコマンドの基盤を作成するヘルパー関数
// (以前の clibase.NewRootCmd のロジックの一部を再実装)
func newRootCmd(appName string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   appName,
		Short: fmt.Sprintf("A CLI tool for %s.", appName),
		Long:  fmt.Sprintf("The CLI tool for %s. Use a subcommand to perform a task.", appName),
		// PersistentPreRunEを手動で実装
		PersistentPreRunE: preRunAppE,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
	// 共通フラグを手動で追加
	rootCmd.PersistentFlags().BoolVarP(&gcbase.Flags.Verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVarP(&gcbase.Flags.ConfigFile, "config", "c", "", "Config file path")

	// アプリケーション固有のフラグを手動で追加
	addAppFlags(rootCmd)

	return rootCmd
}

// addAppFlags は、アプリケーション固有の永続フラグ（--model）を追加します。
func addAppFlags(rootCmd *cobra.Command) {
	defaultModel := "gemini-2.5-flash"
	rootCmd.PersistentFlags().StringVarP(&model, "model", "", defaultModel,
		"使用する Google Gemini モデル名 (例: gemini-2.5-flash, gemini-2.5-pro)")
}

// preRunAppE は、アプリケーション固有の実行前チェック（GEMINI_API_KEY）を実行します。
// これは rootCmd の PersistentPreRunE として機能します。
func preRunAppE(cmd *cobra.Command, args []string) error {
	// 1. clibase 共通の PersistentPreRun 処理をここで実行 (clibase.Executeがしてくれないため)
	if gcbase.Flags.Verbose {
		fmt.Println("Verbose mode enabled by clibase (manual check).")
	}

	// 2. GEMINI_API_KEY の必須チェック
	if os.Getenv("GEMINI_API_KEY") == "" {
		return fmt.Errorf("エラー: 環境変数 GEMINI_API_KEY が設定されていません。Gemini APIの利用には必須です")
	}
	return nil
}

// Execute は、アプリケーションのメインエントリポイントです。
func Execute() {
	appName := "prototypus-ai-doc"

	// 1. サブコマンドのフラグ定義とサブコマンドリストの作成
	var cmds []*cobra.Command
	if generateCmd != nil {
		// フラグ定義をcobraが読み取る前に実行
		initCmdFlags()
		cmds = append(cmds, generateCmd)
	}

	// 2. gcbase.Execute のシグネチャに合わせるため、ルートコマンドを独自に構築し、gcbase.Executeは使用しないか、
	//    gcbase.Execute が root.go で定義されていた元のコードを復元する

	// 今回は、gcbase.Executeがコールバックを受け取らない前提で、gcbase.Executeを使わずにcobraを直接実行します。
	// (gcbaseモジュールのExecuteは、ルートコマンドを構築し、AddCommandを実行するだけのため、手動で再現します。)
	rootCmd := newRootCmd(appName)
	rootCmd.AddCommand(cmds...)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
