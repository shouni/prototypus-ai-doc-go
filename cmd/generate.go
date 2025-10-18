package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"text/template"

	"github.com/spf13/cobra"

	"prototypus-ai-doc-go/internal/ioutils"
	"prototypus-ai-doc-go/internal/poster"
	promptInternal "prototypus-ai-doc-go/internal/prompt"
	"prototypus-ai-doc-go/internal/voicevox"
	"prototypus-ai-doc-go/internal/web"

	geminiClient "github.com/shouni/go-ai-client/pkg/ai/gemini"
)

const MinContentLength = 10

// generateCmd のフラグ変数を定義
var (
	outputFile     string
	mode           string
	postAPI        bool
	voicevoxOutput string
	scriptURL      string
	scriptFile     string

	// AI クライアント設定フラグ
	aiAPIKey string
	aiModel  string
	aiURL    string
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

	// --- フラグ定義 ---
	generateCmd.Flags().StringVarP(&scriptURL, "script-url", "u", "", "Webページからコンテンツを取得するためのURL (例: https://example.com/article)。")
	generateCmd.Flags().StringVarP(&scriptFile, "script-file", "f", "", "入力スクリプトファイルのパス ('-'を指定すると標準入力から読み込みます)。")
	generateCmd.Flags().StringVarP(&outputFile, "output-file", "o", "",
		"生成されたスクリプトを保存するファイルのパス。省略時は標準出力 (stdout) に出力します。")
	generateCmd.Flags().StringVarP(&mode, "mode", "m", "solo",
		"スクリプト生成モード。'dialogue', 'solo', 'duet' などを指定します。")
	generateCmd.Flags().BoolVarP(&postAPI, "post-api", "p", false,
		"生成されたスクリプトを外部APIに投稿します。")
	generateCmd.Flags().StringVarP(&voicevoxOutput, "voicevox", "v", "",
		"生成されたスクリプトをVOICEVOXエンジンで合成し、指定されたファイル名に出力します (例: output.wav)。")

	// AI クライアント設定フラグ
	generateCmd.Flags().StringVar(&aiAPIKey, "ai-api-key", "",
		"Google Gemini APIキー。環境変数 GEMINI_API_KEY を上書きします。")
	generateCmd.Flags().StringVar(&aiModel, "ai-model", "gemini-2.5-flash",
		"使用するGeminiモデル名。")
	// 🚨 修正: ライブラリでサポートされていないため、使用できないことを明記
	generateCmd.Flags().StringVar(&aiURL, "ai-url", "",
		"Gemini APIのベースURL。現在のライブラリでは、このフラグによるAPIエンドポイントのカスタマイズはサポートされていません。")
}

// readFileContent は指定されたファイルパスからコンテンツを読み込みます。
func readFileContent(filePath string) ([]byte, error) {
	fmt.Printf("ファイルから読み込み中: %s\n", filePath)
	return os.ReadFile(filePath)
}

// resolveAPIKey は環境変数とフラグからAPIキーを決定します。
func resolveAPIKey(flagKey string) string {
	if flagKey != "" {
		return flagKey
	}
	// 環境変数 GOOGLE_API_KEY もサポートされているため、両方チェックする
	if os.Getenv("GEMINI_API_KEY") != "" {
		return os.Getenv("GEMINI_API_KEY")
	}
	return os.Getenv("GOOGLE_API_KEY")
}

// runGenerate は generate コマンドの実行ロジックです。
func runGenerate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	if voicevoxOutput != "" && outputFile != "" {
		return fmt.Errorf("voicevox出力(-v)とファイル出力(-o)は同時に指定できません。どちらか一方のみ指定してください")
	}

	// --- 1. 入力元から文章を読み込む ---
	var inputContent []byte
	var err error

	switch {
	case scriptURL != "":
		fmt.Printf("URLからコンテンツを取得中: %s\n", scriptURL)
		var text string
		var hasBodyFound bool
		text, hasBodyFound, err = web.FetchAndExtractText(scriptURL, ctx)
		if err != nil {
			return fmt.Errorf("URLからのコンテンツ取得に失敗しました: %w", err)
		}
		if !hasBodyFound {
			fmt.Fprintf(os.Stderr, "警告: 記事本文が見つかりませんでした。タイトルのみで処理を続行します。\n")
		}
		inputContent = []byte(text)

	case scriptFile != "":
		if scriptFile == "-" {
			fmt.Println("標準入力 (stdin) から読み込み中...")
			inputContent, err = io.ReadAll(os.Stdin)
		} else {
			inputContent, err = readFileContent(scriptFile)
		}
		if err != nil {
			return fmt.Errorf("スクリプトファイル '%s' の読み込みに失敗しました: %w", scriptFile, err)
		}

	default:
		fmt.Println("標準入力 (stdin) から読み込み中...")
		inputContent, err = io.ReadAll(os.Stdin)
		if err != nil {
			if errors.Is(err, io.EOF) && len(inputContent) == 0 {
				return fmt.Errorf("標準入力が空です。文章を入力してください。")
			}
			return fmt.Errorf("標準入力の読み込み中に予期せぬエラーが発生しました: %w", err)
		}
	}

	if len(inputContent) < MinContentLength {
		return fmt.Errorf("入力されたコンテンツが短すぎます (最低%dバイト必要です)。", MinContentLength)
	}

	// --- 2. AIクライアントの初期化とスクリプト生成 ---

	finalAPIKey := resolveAPIKey(aiAPIKey)

	if finalAPIKey == "" {
		return errors.New("AI APIキーが設定されていません。環境変数 GEMINI_API_KEY またはフラグ --ai-api-key を確認してください。")
	}

	// 🚨 修正: NewClientFromEnvではなく、APIキーを直接Configに詰めてNewClientを呼び出す
	clientConfig := geminiClient.Config{
		APIKey: finalAPIKey,
		// MaxRetries はデフォルトを使用（設定可能なフラグがないため）
	}

	// aiURLフラグは、ライブラリの制約により無視されます。
	if aiURL != "" {
		fmt.Fprintf(os.Stderr, "警告: '--ai-url' フラグは現在のライブラリ構造により無視されます。\n")
	}

	aiClient, err := geminiClient.NewClient(ctx, clientConfig)
	if err != nil {
		return fmt.Errorf("AIクライアントの初期化に失敗しました: %w", err)
	}

	fmt.Printf("--- 処理開始 ---\nモード: %s\nモデル: %s\n入力サイズ: %d bytes\n\n", mode, aiModel, len(inputContent))
	fmt.Println("AIによるスクリプト生成を開始します...")

	// プロンプトテンプレート文字列を取得
	promptTemplateString, err := promptInternal.GetPromptByMode(mode)
	if err != nil {
		return fmt.Errorf("プロンプトテンプレートの取得に失敗しました: %w", err)
	}

	// text/template を使用して、テンプレート変数をユーザー入力で置換する
	type InputData struct{ InputText string }
	data := InputData{InputText: string(inputContent)}

	tmpl, err := template.New("prompt").Parse(promptTemplateString)
	if err != nil {
		return fmt.Errorf("プロンプトテンプレートの解析エラー: %w", err)
	}

	var fullPrompt bytes.Buffer
	if err := tmpl.Execute(&fullPrompt, data); err != nil {
		return fmt.Errorf("プロンプトへの入力埋め込みエラー: %w", err)
	}

	promptContentBytes := fullPrompt.Bytes()

	// GenerateContent を呼び出す
	generatedResponse, err := aiClient.GenerateContent(ctx, promptContentBytes, "", aiModel)
	if err != nil {
		return fmt.Errorf("スクリプト生成に失敗しました: %w", err)
	}

	generatedScript := generatedResponse.Text

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

		fmt.Fprintln(os.Stderr, "VOICEVOXスタイルデータをロード中...")
		speakerData, err := voicevox.LoadSpeakers(ctx, voicevoxAPIURL)
		if err != nil {
			return fmt.Errorf("VOICEVOXスタイルデータのロードに失敗しました: %w", err)
		}
		fmt.Fprintln(os.Stderr, "VOICEVOXスタイルデータのロード完了。")

		fmt.Fprintf(os.Stderr, "VOICEVOXエンジンに接続し、音声合成を開始します (出力: %s)...\n", voicevoxOutput)

		// AIが生成したスクリプトがVOICEVOXの期待するタグ形式になっている必要があります。
		err = voicevox.PostToEngine(ctx, generatedScript, voicevoxOutput, speakerData, voicevoxAPIURL)

		if err != nil {
			return fmt.Errorf("音声合成パイプラインの実行に失敗しました: %w", err)
		}
		fmt.Fprintln(os.Stderr, "VOICEVOXによる音声合成が完了し、ファイルに保存されました。")

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

			if len(inputStr) > 0 {
				preview := inputStr
				if len(inputStr) > maxLen {
					preview = inputStr[:maxLen] + "..."
				}
				title = fmt.Sprintf("Generated Script (Stdin): %s", preview)
			} else {
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
