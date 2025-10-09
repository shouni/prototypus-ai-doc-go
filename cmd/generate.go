package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// generateCmd のフラグ変数を定義
var (
	inputFile  string
	outputFile string
	mode       string
)

// generateCmd はナレーションスクリプト生成のメインコマンドです。
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "文章を読み込み、ずんだもん/めたんの対話スクリプトを生成します。",
	Long: `
'generate' コマンドは、入力された文章を Gemini API に送り、
指定されたモード（dialogue/solo）に基づいて整形されたナレーションスクリプトを生成します。

入力元を指定しない場合、標準入力 (stdin) から文章を読み込みます。
出力先を指定しない場合、標準出力 (stdout) に結果を出力します。
`,
	RunE: runGenerate,
}

func init() {
	// generateCmd をルートコマンドに追加
	rootCmd.AddCommand(generateCmd)

	// generateCmd 固有のフラグを定義

	// -i, --input-file フラグ
	generateCmd.Flags().StringVarP(&inputFile, "input-file", "i", "",
		"元となる文章が書かれたファイルのパス。省略時は標準入力 (stdin) を使用します。")

	// -o, --output-file フラグ
	generateCmd.Flags().StringVarP(&outputFile, "output-file", "o", "",
		"生成されたスクリプトの出力ファイル名 (例: out/script.md)。省略時は標準出力 (stdout) に出力します。")

	// -m, --mode フラグ (PersistentFlagsはroot.goで定義済みですが、ここではコマンド固有のフラグとして再定義することも可能です)
	// ただし、今回はroot.goで定義した 'model' を利用し、このコマンドではナレーションモードを定義します
	generateCmd.Flags().StringVarP(&mode, "mode", "m", "dialogue",
		"スクリプト生成モードを指定: 'dialogue' (ずんだもん/めたん対話) または 'solo' (ずんだもんモノローグ)")

	// TODO: Slack通知フラグを後で追加
}

// runGenerate は generate コマンドの実行ロジックです。
func runGenerate(cmd *cobra.Command, args []string) error {
	// 1. 入力元から文章を読み込む
	inputContent, err := readInput(inputFile)
	if err != nil {
		return err
	}

	// 2. 読み込んだ文章をAIに渡す（この部分は次のステップで実装）
	fmt.Printf("--- 入力成功 ---\nモード: %s\nモデル: %s\n入力サイズ: %d bytes\n\n", mode, model, len(inputContent))
	fmt.Println("AIによるスクリプト生成を開始します...")

	// ----------------------------------------------------
	// TODO: pkg/ai/client.go のロジックを呼び出す
	// generatedScript, err := ai.GenerateScript(inputContent, mode, model)
	// if err != nil { return err }
	// ----------------------------------------------------

	// 3. 結果を出力先へ書き出す（現在はテスト用のメッセージを出力）
	testScript := fmt.Sprintf("# 生成スクリプト（テスト出力）\n\n[ずんだもん] %sモードで文章を解析したのだ！\n\n[めたん] 入力コンテンツのサイズは %d バイトでしたわ！", mode, len(inputContent))

	if err := writeOutput(outputFile, testScript); err != nil {
		return err
	}

	return nil
}

// readInput は、ファイルまたは標準入力から内容を読み込みます。
func readInput(filename string) ([]byte, error) {
	if filename != "" {
		// ファイルから読み込み
		fmt.Printf("ファイルから読み込み中: %s\n", filename)
		return os.ReadFile(filename)
	}

	// 標準入力から読み込み
	fmt.Println("標準入力 (stdin) から読み込み中...")
	return io.ReadAll(os.Stdin)
}

// writeOutput は、ファイルまたは標準出力に内容を書き出します。
func writeOutput(filename string, content string) error {
	if filename != "" {
		// ファイルに書き出し
		fmt.Printf("\n--- スクリプト生成完了 ---\nファイルに書き込みました: %s\n", filename)
		return os.WriteFile(filename, []byte(content), 0644)
	}

	// 標準出力に書き出し
	fmt.Println("\n--- スクリプト生成結果 ---")
	fmt.Println(content)
	return nil
}
