package cmd

import (
	"bytes"
	"context"
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

// GenerateOptions はコマンドラインフラグを保持する構造体です。
type GenerateOptions struct {
	OutputFile     string
	Mode           string
	PostAPI        bool
	VoicevoxOutput string
	ScriptURL      string
	ScriptFile     string
	AIAPIKey       string
	AIModel        string
	AIURL          string // ライブラリの制約により無視される
}

// GenerateHandler は generate コマンドの実行に必要な依存とオプションを保持します。
type GenerateHandler struct {
	Options GenerateOptions
}

// グローバルなオプションインスタンス。init() と RunE の間で値を共有するために使用します。
var opts GenerateOptions

// generateCmd はナレーションスクリプト生成のメインコマンドです。
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "AIにナレーションスクリプトを生成させます。",
	Long: `AIに渡す元となる文章を指定し、ナレーションスクリプトを生成します。
Webページやファイル、標準入力から文章を読み込むことができます。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		handler := GenerateHandler{Options: opts}
		return handler.runGenerate(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)

	// --- フラグ定義: グローバル変数ではなく、opts 構造体のフィールドにバインドする ---
	generateCmd.Flags().StringVarP(&opts.ScriptURL, "script-url", "u", "", "Webページからコンテンツを取得するためのURL。")
	generateCmd.Flags().StringVarP(&opts.ScriptFile, "script-file", "f", "", "入力スクリプトファイルのパス ('-'を指定すると標準入力から読み込みます)。")
	generateCmd.Flags().StringVarP(&opts.OutputFile, "output-file", "o", "",
		"生成されたスクリプトを保存するファイルのパス。省略時は標準出力 (stdout) に出力します。")
	generateCmd.Flags().StringVarP(&opts.Mode, "mode", "m", "solo",
		"スクリプト生成モード。'dialogue', 'solo', 'duet' などを指定します。")
	generateCmd.Flags().BoolVarP(&opts.PostAPI, "post-api", "p", false,
		"生成されたスクリプトを外部APIに投稿します。")
	generateCmd.Flags().StringVarP(&opts.VoicevoxOutput, "voicevox", "v", "",
		"生成されたスクリプトをVOICEVOXエンジンで合成し、指定されたファイル名に出力します (例: output.wav)。")

	// AI クライアント設定フラグ
	generateCmd.Flags().StringVar(&opts.AIAPIKey, "ai-api-key", "",
		"Google Gemini APIキー。環境変数 GEMINI_API_KEY を上書きします。")
	generateCmd.Flags().StringVar(&opts.AIModel, "ai-model", "gemini-2.5-flash",
		"使用するGeminiモデル名。")
	generateCmd.Flags().StringVar(&opts.AIURL, "ai-url", "",
		"Gemini APIのベースURL。現在のライブラリでは、このフラグによるAPIエンドポイントのカスタマイズはサポートされていません。")
}

// readFileContent は指定されたファイルパスからコンテンツを読み込みます。（変更なし）
func readFileContent(filePath string) ([]byte, error) {
	fmt.Printf("ファイルから読み込み中: %s\n", filePath)
	return os.ReadFile(filePath)
}

// resolveAPIKey は環境変数とフラグからAPIキーを決定します。（変更なし）
func resolveAPIKey(flagKey string) string {
	if flagKey != "" {
		return flagKey
	}
	if os.Getenv("GEMINI_API_KEY") != "" {
		return os.Getenv("GEMINI_API_KEY")
	}
	return os.Getenv("GOOGLE_API_KEY")
}

// --------------------------------------------------------------------------------
// 責務を分割したヘルパー関数
// --------------------------------------------------------------------------------

// readInputContent は入力ソースからコンテンツを読み込みます。
func (h *GenerateHandler) readInputContent(ctx context.Context) ([]byte, error) {
	if h.Options.VoicevoxOutput != "" && h.Options.OutputFile != "" {
		return nil, fmt.Errorf("voicevox出力(-v)とファイル出力(-o)は同時に指定できません。どちらか一方のみ指定してください")
	}

	var inputContent []byte
	var err error

	switch {
	case h.Options.ScriptURL != "":
		fmt.Printf("URLからコンテンツを取得中: %s\n", h.Options.ScriptURL)
		var text string
		var hasBodyFound bool
		text, hasBodyFound, err = web.FetchAndExtractText(h.Options.ScriptURL, ctx)
		if err != nil {
			return nil, fmt.Errorf("URLからのコンテンツ取得に失敗しました: %w", err)
		}
		if !hasBodyFound {
			fmt.Fprintf(os.Stderr, "警告: 記事本文が見つかりませんでした。タイトルのみで処理を続行します。\n")
		}
		inputContent = []byte(text)

	case h.Options.ScriptFile != "":
		if h.Options.ScriptFile == "-" {
			fmt.Println("標準入力 (stdin) から読み込み中...")
			inputContent, err = io.ReadAll(os.Stdin)
		} else {
			inputContent, err = readFileContent(h.Options.ScriptFile)
		}
		if err != nil {
			return nil, fmt.Errorf("スクリプトファイル '%s' の読み込みに失敗しました: %w", h.Options.ScriptFile, err)
		}

	default:
		fmt.Println("標準入力 (stdin) から読み込み中...")
		inputContent, err = io.ReadAll(os.Stdin)
		if err != nil {
			if errors.Is(err, io.EOF) && len(inputContent) == 0 {
				return nil, fmt.Errorf("標準入力が空です。文章を入力してください。")
			}
			return nil, fmt.Errorf("標準入力の読み込み中に予期せぬエラーが発生しました: %w", err)
		}
	}

	if len(inputContent) < MinContentLength {
		return nil, fmt.Errorf("入力されたコンテンツが短すぎます (最低%dバイト必要です)。", MinContentLength)
	}

	return inputContent, nil
}

// initializeAIClient は AI クライアントを初期化します。
func (h *GenerateHandler) initializeAIClient(ctx context.Context) (*geminiClient.Client, error) {
	finalAPIKey := resolveAPIKey(h.Options.AIAPIKey)

	if finalAPIKey == "" {
		return nil, errors.New("AI APIキーが設定されていません。環境変数 GEMINI_API_KEY またはフラグ --ai-api-key を確認してください。")
	}

	clientConfig := geminiClient.Config{
		APIKey: finalAPIKey,
	}

	if h.Options.AIURL != "" {
		fmt.Fprintf(os.Stderr, "警告: '--ai-url' フラグは現在のライブラリ構造により無視されます。\n")
	}

	aiClient, err := geminiClient.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("AIクライアントの初期化に失敗しました: %w", err)
	}
	return aiClient, nil
}

// buildFullPrompt はプロンプトテンプレートを構築し、入力内容を埋め込みます。
func (h *GenerateHandler) buildFullPrompt(inputContent []byte) ([]byte, error) {
	promptTemplateString, err := promptInternal.GetPromptByMode(h.Options.Mode)
	if err != nil {
		return nil, fmt.Errorf("プロンプトテンプレートの取得に失敗しました: %w", err)
	}

	type InputData struct{ InputText string }
	data := InputData{InputText: string(inputContent)}

	tmpl, err := template.New("prompt").Parse(promptTemplateString)
	if err != nil {
		return nil, fmt.Errorf("プロンプトテンプレートの解析エラー: %w", err)
	}

	var fullPrompt bytes.Buffer
	if err := tmpl.Execute(&fullPrompt, data); err != nil {
		return nil, fmt.Errorf("プロンプトへの入力埋め込みエラー: %w", err)
	}

	return fullPrompt.Bytes(), nil
}

// handleVoicevoxOutput は VOICEVOX 処理を実行し、結果を出力します。
func (h *GenerateHandler) handleVoicevoxOutput(ctx context.Context, generatedScript string) error {
	if h.Options.VoicevoxOutput == "" {
		return nil
	}

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

	fmt.Fprintf(os.Stderr, "VOICEVOXエンジンに接続し、音声合成を開始します (出力: %s)...\n", h.Options.VoicevoxOutput)

	err = voicevox.PostToEngine(ctx, generatedScript, h.Options.VoicevoxOutput, speakerData, voicevoxAPIURL)
	if err != nil {
		return fmt.Errorf("音声合成パイプラインの実行に失敗しました: %w", err)
	}

	fmt.Fprintln(os.Stderr, "VOICEVOXによる音声合成が完了し、ファイルに保存されました。")
	return nil
}

// handleFinalOutput はスクリプトをファイルまたは標準出力に出力します。
func (h *GenerateHandler) handleFinalOutput(generatedScript string) error {
	return ioutils.WriteOutput(h.Options.OutputFile, generatedScript)
}

// handlePostAPI は生成されたスクリプトを外部APIに投稿します。
func (h *GenerateHandler) handlePostAPI(inputContent []byte, generatedScript string) error {
	if !h.Options.PostAPI {
		return nil
	}

	title := h.Options.OutputFile
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
			title = fmt.Sprintf("Generated Script (Empty Input) - Mode: %s", h.Options.Mode)
		}
	}

	fmt.Fprintln(os.Stderr, "外部APIに投稿中...")
	if err := poster.PostToAPI(title, h.Options.Mode, generatedScript); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 外部APIへの投稿に失敗しました: %v\n", err)
	} else {
		fmt.Fprintln(os.Stderr, "外部APIへの投稿が完了しました。")
	}

	return nil
}

// --------------------------------------------------------------------------------
// メイン実行ロジック (簡素化)
// --------------------------------------------------------------------------------

// runGenerate は generate コマンドの実行ロジックです。
func (h *GenerateHandler) runGenerate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// 1. 入力元から文章を読み込む
	inputContent, err := h.readInputContent(ctx)
	if err != nil {
		return err
	}

	// 2. AIクライアントの初期化
	aiClient, err := h.initializeAIClient(ctx)
	if err != nil {
		return err
	}

	// ログ出力
	fmt.Printf("--- 処理開始 ---\nモード: %s\nモデル: %s\n入力サイズ: %d bytes\n\n", h.Options.Mode, h.Options.AIModel, len(inputContent))
	fmt.Println("AIによるスクリプト生成を開始します...")

	// 3. プロンプトの構築
	promptContentBytes, err := h.buildFullPrompt(inputContent)
	if err != nil {
		return err
	}

	// 4. AIによるスクリプト生成
	generatedResponse, err := aiClient.GenerateContent(ctx, promptContentBytes, "", h.Options.AIModel)
	if err != nil {
		return fmt.Errorf("スクリプト生成に失敗しました: %w", err)
	}
	generatedScript := generatedResponse.Text

	// 生成されたスクリプトを標準エラー出力に進捗メッセージとして表示
	fmt.Fprintln(os.Stderr, "\n--- AI スクリプト生成結果 ---")
	fmt.Fprintln(os.Stderr, generatedScript)
	fmt.Fprintln(os.Stderr, "------------------------------------")

	// 5. VOICEVOX出力の処理 (ここで終了する場合は return)
	if err := h.handleVoicevoxOutput(ctx, generatedScript); err != nil {
		return err
	}
	if h.Options.VoicevoxOutput != "" {
		return nil // VOICEVOX出力が成功した場合、ここで処理を終了
	}

	// 6. 通常のI/O出力
	if err := h.handleFinalOutput(generatedScript); err != nil {
		return err
	}

	// 7. API投稿オプションの処理
	return h.handlePostAPI(inputContent, generatedScript)
}
