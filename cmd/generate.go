package cmd

import (
	"context"
	"errors" // errorsパッケージを追加
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"prototypus-ai-doc-go/internal/ai"
	"prototypus-ai-doc-go/internal/ioutils"
	"prototypus-ai-doc-go/internal/poster"
	"prototypus-ai-doc-go/internal/voicevox"
	"prototypus-ai-doc-go/internal/web"
)

const MinContentLength = 10 // AIに渡す最低限のコンテンツ長 (バイト数)

// generateCmd のフラグ変数を定義
var (
	inputFile      string
	outputFile     string
	mode           string
	postAPI        bool
	voicevoxOutput string
	scriptURL      string
	scriptFile     string
)

// generateCmd はナレーションスクリプト生成のメインコマンドです。
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "AIにナレーションスクリプトを生成させます。",
	Long: `AIに渡す元となる文章を指定し、ナレーションスクリプトを生成します。
Webページやファイル、標準入力から文章を読み込むことができます。`,
	RunE: runGenerate,
}

func init() {
	rootCmd.AddCommand(generateCmd)

	// --- 入力フラグ ---

	// -i, --input-file フラグ (非推奨。互換性のために残す)
	generateCmd.Flags().StringVarP(&inputFile, "input-file", "i", "",
		"元となる文章が書かれたファイルのパス。--script-file に移行されました。")
	generateCmd.Flags().MarkDeprecated("input-file", "use --script-file (-f) instead.")

	// 新しい入力フラグ
	generateCmd.Flags().StringVarP(&scriptURL, "script-url", "u", "", "Webページからコンテンツを取得するためのURL (例: https://example.com/article)。")
	generateCmd.Flags().StringVarP(&scriptFile, "script-file", "f", "", "入力スクリプトファイルのパス ('-'を指定すると標準入力から読み込みます)。")

	// 入力フラグは相互に排他的であるとマーク (Cobraが自動でエラーチェックを行う)
	generateCmd.MarkFlagsMutuallyExclusive("script-url", "script-file", "input-file")

	// --- 出力/設定フラグ ---

	// -o, --output-file フラグ
	generateCmd.Flags().StringVarP(&outputFile, "output-file", "o", "",
		"生成されたスクリプトを保存するファイルのパス。省略時は標準出力 (stdout) に出力します。")

	// -m, --mode フラグ (デフォルト値を "solo" に戻す)
	generateCmd.Flags().StringVarP(&mode, "mode", "m", "solo",
		"スクリプト生成モード。'dialogue', 'solo', 'duet' などを指定します。")

	// -p, --post-api フラグ
	generateCmd.Flags().BoolVarP(&postAPI, "post-api", "p", false,
		"生成されたスクリプトを外部APIに投稿します。")

	// -v, --voicevox フラグの定義
	generateCmd.Flags().StringVarP(&voicevoxOutput, "voicevox", "v", "",
		"生成されたスクリプトをVOICEVOXエンジンで合成し、指定されたファイル名に出力します (例: output.wav)。")
}

// readFileContent は指定されたファイルパスからコンテンツを読み込みます。
func readFileContent(filePath string) ([]byte, error) {
	fmt.Printf("ファイルから読み込み中: %s\n", filePath)
	return os.ReadFile(filePath)
}

// runGenerate は generate コマンドの実行ロジックです。
func runGenerate(cmd *cobra.Command, args []string) error {

	if voicevoxOutput != "" && outputFile != "" {
		return fmt.Errorf("voicevox出力(-v)とファイル出力(-o)は同時に指定できません。どちらか一方のみ指定してください。")
	}

	// --- 1. 入力元から文章を読み込む（switchステートメントで簡素化） ---
	var inputContent []byte
	var err error

	switch {
	case scriptURL != "":
		fmt.Printf("URLからコンテンツを取得中: %s\n", scriptURL)
		var text string
		var hasBodyFound bool

		text, hasBodyFound, err = web.FetchAndExtractText(scriptURL, cmd.Context())
		if err != nil {
			return fmt.Errorf("URLからのコンテンツ取得に失敗しました: %w", err)
		}
		if !hasBodyFound {
			// 警告ログを上位レイヤーで処理
			fmt.Fprintf(os.Stderr, "警告: 記事本文が見つかりませんでした。タイトルのみで処理を続行します。\n")
		}
		inputContent = []byte(text)

	case scriptFile != "":
		if scriptFile == "-" {
			fmt.Println("標準入力 (stdin) から読み込み中...")
			inputContent, err = io.ReadAll(os.Stdin)
		} else {
			inputContent, err = readFileContent(scriptFile) // ヘルパー関数を呼び出す
		}
		if err != nil {
			return fmt.Errorf("スクリプトファイル '%s' の読み込みに失敗しました: %w", scriptFile, err)
		}

	case inputFile != "": // 非推奨フラグだが、互換性のために残す
		inputContent, err = readFileContent(inputFile) // ヘルパー関数を呼び出す
		if err != nil {
			return fmt.Errorf("入力ファイル '%s' の読み込みに失敗しました: %w", inputFile, err)
		}

	default:
		// いずれのフラグも指定なしの場合、標準入力から読み込み
		fmt.Println("標準入力 (stdin) から読み込み中...")
		inputContent, err = io.ReadAll(os.Stdin)
		if err != nil {
			// 標準入力が閉じられたことによるEOFや、その他のI/Oエラーを区別
			if errors.Is(err, io.EOF) && len(inputContent) == 0 {
				return fmt.Errorf("標準入力が空です。文章を入力してください。")
			}
			return fmt.Errorf("標準入力の読み込み中に予期せぬエラーが発生しました: %w", err)
		}
	}

	// 入力チェックを強化
	if len(inputContent) < MinContentLength {
		return fmt.Errorf("入力されたコンテンツが短すぎます (最低%dバイト必要です)。", MinContentLength)
	}

	fmt.Printf("--- 処理開始 ---\nモード: %s\nモデル: %s\n入力サイズ: %d bytes\n\n", mode, model, len(inputContent))
	fmt.Println("AIによるスクリプト生成を開始します...")

	// NewClient を使用してクライアントを初期化
	aiClient, err := ai.NewClient(context.Background(), model)
	if err != nil {
		return fmt.Errorf("AIクライアントの初期化に失敗しました: %w", err)
	}
	defer aiClient.Close() // クライアントを確実に閉じる

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
		voicevoxAPIURL := os.Getenv("VOICEVOX_API_URL")
		if voicevoxAPIURL == "" {
			return fmt.Errorf("VOICEVOX_API_URL 環境変数が設定されていません")
		}

		// VOICEVOXスタイルデータ（話者情報）をロード
		fmt.Fprintln(os.Stderr, "VOICEVOXスタイルデータをロード中...")
		speakerData, err := voicevox.LoadSpeakers(cmd.Context(), voicevoxAPIURL)
		if err != nil {
			return fmt.Errorf("VOICEVOXスタイルデータのロードに失敗しました: %w", err)
		}
		fmt.Fprintln(os.Stderr, "VOICEVOXスタイルデータのロード完了。")
		// ---------------------------------------------

		// VOICEVOX出力が指定されている場合、合成処理を実行
		fmt.Fprintf(os.Stderr, "VOICEVOXエンジンに接続し、音声合成を開始します (出力: %s)...\n", voicevoxOutput)

		// 修正後の呼び出し: speakerDataを引数として渡す
		err = voicevox.PostToEngine(cmd.Context(), generatedScript, voicevoxOutput, speakerData, voicevoxAPIURL)

		if err != nil {
			return fmt.Errorf("音声合成パイプラインの実行に失敗しました: %w", err)
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
