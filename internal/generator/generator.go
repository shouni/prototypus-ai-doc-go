package generator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"text/template"
	"time"

	geminiClient "github.com/shouni/go-ai-client/pkg/ai/gemini"
	webextractor "github.com/shouni/go-web-exact/pkg/web"

	"prototypus-ai-doc-go/internal/ioutils"
	"prototypus-ai-doc-go/internal/poster"
	promptInternal "prototypus-ai-doc-go/internal/prompt"
	"prototypus-ai-doc-go/internal/voicevox"
)

const MinInputContentLength = 10

// --------------------------------------------------------------------------------
// 内部ヘルパー関数 (GenerateHandlerのプライベートメソッドへ移動)
// --------------------------------------------------------------------------------

// readFileContent は指定されたファイルパスからコンテンツを読み込みます。
func (h *GenerateHandler) readFileContent(filePath string) ([]byte, error) {
	fmt.Printf("ファイルから読み込み中: %s\n", filePath)
	return os.ReadFile(filePath)
}

// resolveAPIKey は環境変数とフラグからAPIキーを決定します。
func (h *GenerateHandler) resolveAPIKey(flagKey string) string {
	if flagKey != "" {
		return flagKey
	}
	if os.Getenv("GEMINI_API_KEY") != "" {
		return os.Getenv("GEMINI_API_KEY")
	}
	return os.Getenv("GOOGLE_API_KEY")
}

// --------------------------------------------------------------------------------
// 構造体定義
// --------------------------------------------------------------------------------

// GenerateOptions はコマンドラインフラグを保持する構造体です。
type GenerateOptions struct {
	OutputFile          string
	Mode                string
	PostAPI             bool
	VoicevoxOutput      string
	ScriptURL           string
	ScriptFile          string
	AIAPIKey            string
	AIModel             string
	HTTPTimeout         time.Duration
	VoicevoxFallbackTag string
}

// GenerateHandler は generate コマンドの実行に必要な依存とオプションを保持します。
type GenerateHandler struct {
	Options        GenerateOptions
	Extractor      *webextractor.Extractor
	VoicevoxClient *voicevox.Client
}

// --------------------------------------------------------------------------------
// メイン実行ロジック
// --------------------------------------------------------------------------------

// RunGenerate は generate コマンドの実行ロジックです。
func (h *GenerateHandler) RunGenerate(ctx context.Context) error {
	// 1. 入力元から文章を読み込む
	inputContent, err := h.ReadInputContent(ctx)
	if err != nil {
		return err
	}

	// 2. AIクライアントの初期化
	aiClient, err := h.InitializeAIClient(ctx)
	if err != nil {
		return err
	}

	// ログ出力
	fmt.Printf("--- 処理開始 ---\nモード: %s\nモデル: %s\n入力サイズ: %d bytes\n\n", h.Options.Mode, h.Options.AIModel, len(inputContent))
	fmt.Println("AIによるスクリプト生成を開始します...")

	// 3. プロンプトの構築
	promptContent, err := h.BuildFullPrompt(string(inputContent))
	if err != nil {
		return err
	}

	// 4. AIによるスクリプト生成
	generatedResponse, err := aiClient.GenerateContent(ctx, promptContent, h.Options.AIModel)
	if err != nil {
		return fmt.Errorf("スクリプト生成に失敗しました: %w", err)
	}
	generatedScript := generatedResponse.Text

	// 生成されたスクリプトを標準エラー出力に進捗メッセージとして表示
	fmt.Fprintln(os.Stderr, "\n--- AI スクリプト生成結果 ---")
	fmt.Fprintln(os.Stderr, generatedScript)
	fmt.Fprintln(os.Stderr, "------------------------------------")

	// 5. VOICEVOX出力の処理
	if err := h.HandleVoicevoxOutput(ctx, generatedScript); err != nil {
		return err
	}
	if h.Options.VoicevoxOutput != "" {
		return nil // VOICEVOX出力が成功した場合、ここで処理を終了
	}

	// 6. 通常のI/O出力
	if err := h.HandleFinalOutput(generatedScript); err != nil {
		return err
	}

	// 7. API投稿オプションの処理
	return h.HandlePostAPI(inputContent, generatedScript)
}

// --------------------------------------------------------------------------------
// ヘルパー関数
// --------------------------------------------------------------------------------

// ReadInputContent は入力ソースからコンテンツを読み込みます。
func (h *GenerateHandler) ReadInputContent(ctx context.Context) ([]byte, error) {
	if h.Options.VoicevoxOutput != "" && h.Options.OutputFile != "" {
		return nil, fmt.Errorf("voicevox出力(-v)とファイル出力(-o)は同時に指定できません。どちらか一方のみ指定してください")
	}

	var inputContent []byte
	var err error

	switch {
	case h.Options.ScriptURL != "":
		fmt.Printf("URLからコンテンツを取得中: %s (タイムアウト: %s)\n", h.Options.ScriptURL, h.Options.HTTPTimeout.String())
		var text string
		var hasBodyFound bool

		text, hasBodyFound, err = h.Extractor.FetchAndExtractText(h.Options.ScriptURL, ctx)
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
			// プライベートメソッドを呼び出す
			inputContent, err = h.readFileContent(h.Options.ScriptFile)
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

	if len(inputContent) < MinInputContentLength {
		return nil, fmt.Errorf("入力されたコンテンツが短すぎます (最低%dバイト必要です)。", MinInputContentLength)
	}

	return inputContent, nil
}

// InitializeAIClient は AI クライアントを初期化します。
func (h *GenerateHandler) InitializeAIClient(ctx context.Context) (*geminiClient.Client, error) {
	// プライベートメソッドを呼び出す
	finalAPIKey := h.resolveAPIKey(h.Options.AIAPIKey)

	if finalAPIKey == "" {
		return nil, errors.New("AI APIキーが設定されていません。環境変数 GEMINI_API_KEY またはフラグ --ai-api-key を確認してください。")
	}

	clientConfig := geminiClient.Config{
		APIKey: finalAPIKey,
	}

	aiClient, err := geminiClient.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("AIクライアントの初期化に失敗しました: %w", err)
	}
	return aiClient, nil
}

// BuildFullPrompt はプロンプトテンプレートを構築し、入力内容を埋め込みます。
func (h *GenerateHandler) BuildFullPrompt(inputText string) (string, error) { // 引数をstringに変更
	promptTemplateString, err := promptInternal.GetPromptByMode(h.Options.Mode)
	if err != nil {
		return "", fmt.Errorf("プロンプトテンプレートの取得に失敗しました: %w", err) // 戻り値をstringに合わせる
	}

	type InputData struct{ InputText string }
	data := InputData{InputText: inputText} // stringのまま使用

	tmpl, err := template.New("prompt").Parse(promptTemplateString)
	if err != nil {
		return "", fmt.Errorf("プロンプトテンプレートの解析エラー: %w", err)
	}

	var fullPrompt bytes.Buffer
	if err := tmpl.Execute(&fullPrompt, data); err != nil {
		return "", fmt.Errorf("プロンプトへの入力埋め込みエラー: %w", err)
	}

	return fullPrompt.String(), nil // []byteではなくstringを返す
}

// HandleVoicevoxOutput は VOICEVOX 処理を実行し、結果を出力します。
func (h *GenerateHandler) HandleVoicevoxOutput(ctx context.Context, generatedScript string) error {
	if h.Options.VoicevoxOutput == "" {
		return nil
	}

	// VoicevoxClientが注入されていることを確認
	client := h.VoicevoxClient
	if client == nil {
		return errors.New("内部エラー: VoicevoxClientが初期化されていません")
	}

	fmt.Fprintln(os.Stderr, "VOICEVOXスタイルデータをロード中...")

	speakerData, err := voicevox.LoadSpeakers(ctx, client)
	if err != nil {
		return fmt.Errorf("VOICEVOXスタイルデータのロードに失敗しました: %w", err)
	}
	fmt.Fprintln(os.Stderr, "VOICEVOXスタイルデータのロード完了。")

	fmt.Fprintf(os.Stderr, "VOICEVOXエンジンに接続し、音声合成を開始します (出力: %s)...\n", h.Options.VoicevoxOutput)

	// ★ 修正: voicevox.PostToEngineの呼び出しに新しい引数 (fallbackTag) を追加
	err = voicevox.PostToEngine(
		ctx,
		generatedScript,
		h.Options.VoicevoxOutput,
		speakerData,
		client,
		h.Options.VoicevoxFallbackTag, // ★ 追加された引数
	)
	if err != nil {
		return fmt.Errorf("音声合成パイプラインの実行に失敗しました: %w", err)
	}

	fmt.Fprintln(os.Stderr, "VOICEVOXによる音声合成が完了し、ファイルに保存されました。")
	return nil
}

// HandleFinalOutput はスクリプトをファイルまたは標準出力に出力します。
func (h *GenerateHandler) HandleFinalOutput(generatedScript string) error {
	return ioutils.WriteOutput(h.Options.OutputFile, generatedScript)
}

// HandlePostAPI は生成されたスクリプトを外部APIに投稿します。
func (h *GenerateHandler) HandlePostAPI(inputContent []byte, generatedScript string) error {
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
