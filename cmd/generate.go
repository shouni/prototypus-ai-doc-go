package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"prototypus-ai-doc-go/internal/ai"
	"prototypus-ai-doc-go/internal/ioutils"
	"prototypus-ai-doc-go/internal/poster"
	"prototypus-ai-doc-go/internal/voicevox"
)

// generateCmd のフラグ変数を定義
var (
	inputFile      string
	outputFile     string
	mode           string
	postAPI        bool
	voicevoxOutput string
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

	// -i, --input-file フラグ
	generateCmd.Flags().StringVarP(&inputFile, "input-file", "i", "",
		"元となる文章が書かれたファイルのパス。省略時は標準入力 (stdin) を使用します。")

	// -o, --output-file フラグ
	generateCmd.Flags().StringVarP(&outputFile, "output-file", "o", "",
		"生成されたスクリプトの出力ファイル名 (例: out/script.md)。省略時は標準出力 (stdout) に出力します。")

	// -m, --mode フラグ
	generateCmd.Flags().StringVarP(&mode, "mode", "m", "dialogue",
		"スクリプト生成モードを指定: 'dialogue' (ずんだもん/めたん対話) または 'solo' (ずんだもんモノローグ)")

	// -p, --post-api フラグ
	generateCmd.Flags().BoolVarP(&postAPI, "post-api", "p", false,
		"生成されたスクリプトを外部APIに投稿します (環境変数 POST_API_URL が必要)。")

	// -v, --voicevox フラグの定義
	generateCmd.Flags().StringVarP(&voicevoxOutput, "voicevox", "v", "",
		"生成されたスクリプトをVOICEVOXエンジンで合成し、指定されたファイル名に出力します (例: output.wav)。")
}

// runGenerate は generate コマンドの実行ロジックです。
func runGenerate(cmd *cobra.Command, args []string) error {

	if voicevoxOutput != "" && outputFile != "" {
		return fmt.Errorf("voicevox出力(-v)とファイル出力(-o)は同時に指定できません。どちらか一方のみ指定してください。")
	}

	// 1. 入力元から文章を読み込む
	inputContent, err := readInput(inputFile)
	if err != nil {
		// ファイル読み込み失敗時にエラーメッセージを表示
		return fmt.Errorf("入力ファイルの読み込みに失敗しました: %w", err)
	}

	// 入力チェックを強化
	if len(inputContent) == 0 {
		return fmt.Errorf("エラー: 入力コンテンツが空です。文章を入力してください")
	}

	fmt.Printf("--- 処理開始 ---\nモード: %s\nモデル: %s\n入力サイズ: %d bytes\n\n", mode, model, len(inputContent))
	fmt.Println("AIによるスクリプト生成を開始します...")

	// NewClient を使用してクライアントを初期化
	aiClient, err := ai.NewClient(context.Background(), model)
	if err != nil {
		return fmt.Errorf("AIクライアントの初期化に失敗しました: %w", err)
	}
	defer aiClient.Close() // ★ 修正点2: クライアントを確実に閉じる

	generatedScript, err := aiClient.GenerateScript(context.Background(), inputContent, mode)
	if err != nil {
		return fmt.Errorf("スクリプト生成に失敗しました: %w", err)
	}

	// 生成されたスクリプトを標準エラー出力に進捗メッセージとして表示
	fmt.Fprintln(os.Stderr, "\n--- AI スクリプト生成結果 ---")
	fmt.Fprintln(os.Stderr, generatedScript)
	fmt.Fprintln(os.Stderr, "------------------------------------")

	// 3. VOICEVOX出力の処理
	if voicevoxOutput != "" {
		// VOICEVOX出力が指定されている場合、合成処理を実行
		fmt.Fprintf(os.Stderr, "VOICEVOXエンジンに接続し、音声合成を開始します (出力: %s)...\n", voicevoxOutput)

		if err := voicevox.PostToEngine(generatedScript, voicevoxOutput); err != nil {
			return fmt.Errorf("VOICEVOX音声合成に失敗しました: %w", err)
		}
		fmt.Fprintln(os.Stderr, "VOICEVOXによる音声合成が完了し、ファイルに保存されました。")

		// 音声ファイルが出力されたため、ここで処理を終了
		return nil
	}

	// 4. 通常のI/O出力 (voicevoxOutput が空の場合のみ実行)
	if err := ioutils.WriteOutput(outputFile, generatedScript); err != nil {
		return fmt.Errorf("出力ファイルへの書き込みに失敗しました: %w", err)
	}

	// 4. API投稿オプションの処理
	if postAPI {
		title := outputFile
		if title == "" {
			const maxLen = 50
			inputStr := string(inputContent)

			// 入力コンテンツの冒頭を使用
			if len(inputStr) > 0 {
				preview := inputStr
				if len(inputStr) > maxLen {
					preview = inputStr[:maxLen] + "..."
				}
				title = fmt.Sprintf("Generated Script (Stdin): %s", preview)
			} else {
				// 入力が空の場合は、モードをタイトルにする
				title = fmt.Sprintf("Generated Script (Empty Input) - Mode: %s", mode)
			}
		}

		fmt.Fprintln(os.Stderr, "外部APIに投稿中...")
		if err := poster.PostToAPI(title, mode, generatedScript); err != nil {
			fmt.Fprintf(os.Stderr, "警告: 外部APIへの投稿に失敗しました: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "外部APIへの投稿が完了しました。")
		}
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
