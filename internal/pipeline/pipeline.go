package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"text/template"
	"time"

	"prototypus-ai-doc-go/internal/ioutils"
	"prototypus-ai-doc-go/internal/poster"
	"prototypus-ai-doc-go/internal/prompt"

	"github.com/shouni/go-ai-client/v2/pkg/ai/gemini"
	"github.com/shouni/go-voicevox/pkg/voicevox"
	"github.com/shouni/go-web-exact/v2/pkg/extract"
)

const MinInputContentLength = 10

// --------------------------------------------------------------------------------
// 構造体定義 (変更なし)
// --------------------------------------------------------------------------------

// GenerateOptions はコマンドラインフラグを保持する構造体です。
type GenerateOptions struct {
	OutputFile          string
	Mode                string
	PostAPI             bool
	VoicevoxOutput      string
	ScriptURL           string
	ScriptFile          string
	AIModel             string
	HTTPTimeout         time.Duration
	VoicevoxFallbackTag string
}

// GenerateHandler は generate コマンドの実行に必要な依存とオプションを保持します。
type GenerateHandler struct {
	Options        GenerateOptions
	Extractor      *extract.Extractor
	AiClient       *gemini.Client
	VoicevoxClient *voicevox.Client
}

// --------------------------------------------------------------------------------
// メイン実行ロジック (RunGenerate)
// --------------------------------------------------------------------------------

// RunGenerate は generate コマンドの実行ロジックです。
func (h *GenerateHandler) RunGenerate(ctx context.Context) error {
	// 1. 入力元から文章を読み込む
	inputContent, err := h.readInputContent(ctx)
	if err != nil {
		return err
	}

	// ログ出力
	fmt.Printf("--- 処理開始 ---\nモード: %s\nモデル: %s\n入力サイズ: %d bytes\n\n", h.Options.Mode, h.Options.AIModel, len(inputContent))
	fmt.Println("AIによるスクリプト生成を開始します...")

	// 2. プロンプトの構築
	promptContent, err := h.buildFullPrompt(string(inputContent))
	if err != nil {
		return err
	}

	// 3. AIによるスクリプト生成
	generatedResponse, err := h.AiClient.GenerateContent(ctx, promptContent, h.Options.AIModel)
	if err != nil {
		return fmt.Errorf("スクリプト生成に失敗しました: %w", err)
	}
	generatedScript := generatedResponse.Text

	// 生成されたスクリプトを標準エラー出力に進捗メッセージとして表示
	fmt.Fprintln(os.Stderr, "\n--- AI スクリプト生成結果 ---")
	fmt.Fprintln(os.Stderr, generatedScript)
	fmt.Fprintln(os.Stderr, "------------------------------------")

	// 4. VOICEVOX出力の処理
	if err := h.handleVoicevoxOutput(ctx, generatedScript); err != nil {
		return err
	}
	if h.Options.VoicevoxOutput != "" {
		return nil // VOICEVOX出力が成功した場合、ここで処理を終了
	}

	// 5. 通常のI/O出力
	if err := h.handleFinalOutput(generatedScript); err != nil {
		return err
	}

	// 6. API投稿オプションの処理
	return h.handlePostAPI(inputContent, generatedScript)
}

// --------------------------------------------------------------------------------
// ヘルパー関数 (入力処理)
// --------------------------------------------------------------------------------

// readFileContent は指定されたファイルパスからコンテンツを読み込みます。
func (h *GenerateHandler) readFileContent(filePath string) ([]byte, error) {
	fmt.Printf("ファイルから読み込み中: %s\n", filePath)
	return os.ReadFile(filePath)
}

// readFromURL はURLからコンテンツを取得します。
func (h *GenerateHandler) readFromURL(ctx context.Context) ([]byte, error) {
	fmt.Printf("URLからコンテンツを取得中: %s (タイムアウト: %s)\n", h.Options.ScriptURL, h.Options.HTTPTimeout.String())

	text, hasBodyFound, err := h.Extractor.FetchAndExtractText(h.Options.ScriptURL, ctx)
	if err != nil {
		return nil, fmt.Errorf("URLからのコンテンツ取得に失敗しました: %w", err)
	}
	if !hasBodyFound {
		fmt.Fprintf(os.Stderr, "警告: 記事本文が見つかりませんでした。タイトルのみで処理を続行します。\n")
	}
	return []byte(text), nil
}

// readFromFile はファイルまたは標準入力からコンテンツを読み込みます。
func (h *GenerateHandler) readFromFile() ([]byte, error) {
	if h.Options.ScriptFile == "-" {
		fmt.Println("標準入力 (stdin) から読み込み中...")
		content, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("標準入力の読み込み中に予期せぬエラーが発生しました: %w", err)
		}
		return content, nil
	}

	content, err := h.readFileContent(h.Options.ScriptFile)
	if err != nil {
		return nil, fmt.Errorf("スクリプトファイル '%s' の読み込みに失敗しました: %w", h.Options.ScriptFile, err)
	}
	return content, nil
}

// readFromStdin は引数なしの標準入力からの読み込みを処理します。
func (h *GenerateHandler) readFromStdin() ([]byte, error) {
	fmt.Println("標準入力 (stdin) から読み込み中...")
	inputContent, err := io.ReadAll(os.Stdin)
	if err != nil {
		if errors.Is(err, io.EOF) && len(inputContent) == 0 {
			return nil, fmt.Errorf("標準入力が空です。文章を入力してください。")
		}
		return nil, fmt.Errorf("標準入力の読み込み中に予期せぬエラーが発生しました: %w", err)
	}
	return inputContent, nil
}

// readInputContent は入力ソースからコンテンツを読み込みます。
func (h *GenerateHandler) readInputContent(ctx context.Context) ([]byte, error) {
	// 早期リターン条件チェック
	if h.Options.VoicevoxOutput != "" && h.Options.OutputFile != "" {
		return nil, fmt.Errorf("voicevox出力(-v)とファイル出力(-o)は同時に指定できません。どちらか一方のみ指定してください")
	}

	var inputContent []byte
	var err error

	switch {
	case h.Options.ScriptURL != "":
		inputContent, err = h.readFromURL(ctx)
	case h.Options.ScriptFile != "":
		inputContent, err = h.readFromFile()
	default:
		inputContent, err = h.readFromStdin()
	}

	if err != nil {
		return nil, err
	}

	if len(inputContent) < MinInputContentLength {
		return nil, fmt.Errorf("入力されたコンテンツが短すぎます (最低%dバイト必要です)。", MinInputContentLength)
	}

	return inputContent, nil
}

// --------------------------------------------------------------------------------
// ヘルパー関数 (AI/VOICEVOX処理)
// --------------------------------------------------------------------------------

// buildFullPrompt はプロンプトテンプレートを構築し、入力内容を埋め込みます。
func (h *GenerateHandler) buildFullPrompt(inputText string) (string, error) {
	promptTemplateString, err := prompt.GetPromptByMode(h.Options.Mode)
	if err != nil {
		return "", fmt.Errorf("プロンプトテンプレートの取得に失敗しました: %w", err)
	}

	type InputData struct{ InputText string }
	data := InputData{InputText: inputText}

	tmpl, err := template.New("prompt").Parse(promptTemplateString)
	if err != nil {
		return "", fmt.Errorf("プロンプトテンプレートの解析エラー: %w", err)
	}

	var fullPrompt bytes.Buffer
	if err := tmpl.Execute(&fullPrompt, data); err != nil {
		return "", fmt.Errorf("プロンプトへの入力埋め込みエラー: %w", err)
	}

	return fullPrompt.String(), nil
}

// loadVoicevoxSpeakerData は VOICEVOX スタイルデータをロードします。
func (h *GenerateHandler) loadVoicevoxSpeakerData(ctx context.Context) (*voicevox.SpeakerData, error) {
	fmt.Fprintln(os.Stderr, "VOICEVOXスタイルデータをロード中...")

	// HTTPTimeout をロード処理のコンテキストタイムアウトとして使用
	loadCtx, cancel := context.WithTimeout(ctx, h.Options.HTTPTimeout)
	defer cancel()

	speakerData, err := voicevox.LoadSpeakers(loadCtx, h.VoicevoxClient)
	if err != nil {
		return nil, fmt.Errorf("VOICEVOXスタイルデータのロードに失敗しました: %w", err)
	}

	// 💡 修正 L198: 成功した場合にのみ完了メッセージを出力
	fmt.Fprintln(os.Stderr, "VOICEVOXスタイルデータのロード完了。")
	return speakerData, nil
}

// handleVoicevoxOutput は VOICEVOX 処理を実行し、結果を出力します。
func (h *GenerateHandler) handleVoicevoxOutput(ctx context.Context, generatedScript string) error {
	if h.Options.VoicevoxOutput == "" {
		return nil
	}

	if h.VoicevoxClient == nil {
		return errors.New("内部エラー: VoicevoxClientが初期化されていません")
	}

	speakerData, err := h.loadVoicevoxSpeakerData(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "VOICEVOXエンジンに接続し、音声合成を開始します (出力: %s)...\n", h.Options.VoicevoxOutput)

	// パーサーの初期化と Engine への依存性注入
	parser := voicevox.NewTextParser()
	engine := voicevox.NewEngine(h.VoicevoxClient, speakerData, parser)

	// Execute処理は時間がかかる可能性があるため、RunGenerateで受け取ったコンテキスト(ctx)を使用
	err = engine.Execute(ctx, generatedScript, h.Options.VoicevoxOutput, h.Options.VoicevoxFallbackTag)

	if err != nil {
		return fmt.Errorf("音声合成パイプラインの実行に失敗しました: %w", err)
	}

	fmt.Fprintln(os.Stderr, "VOICEVOXによる音声合成が完了し、ファイルに保存されました。")
	return nil
}

// --------------------------------------------------------------------------------
// ヘルパー関数 (出力処理)
// --------------------------------------------------------------------------------

// handleFinalOutput はスクリプトをファイルまたは標準出力に出力します。
func (h *GenerateHandler) handleFinalOutput(generatedScript string) error {
	return ioutils.WriteOutput(h.Options.OutputFile, generatedScript)
}

// generatePostTitle は API 投稿用のタイトルを生成します。
func (h *GenerateHandler) generatePostTitle(inputContent []byte) string {
	if h.Options.OutputFile != "" {
		// OutputFileオプションを投稿タイトルとして再利用 (cmd/generate.goで定義された機能)
		return h.Options.OutputFile
	}

	inputStr := string(inputContent)

	if len(inputStr) == 0 {
		return fmt.Sprintf("Generated Script (Empty Input) - Mode: %s", h.Options.Mode)
	}

	const maxLen = 50
	preview := inputStr
	if len(inputStr) > maxLen {
		preview = inputStr[:maxLen] + "..."
	}

	return fmt.Sprintf("Generated Script (Stdin/File Preview): %s", preview)
}

// handlePostAPI は生成されたスクリプトを外部APIに投稿します。
func (h *GenerateHandler) handlePostAPI(inputContent []byte, generatedScript string) error {
	if !h.Options.PostAPI {
		return nil
	}

	title := h.generatePostTitle(inputContent)

	fmt.Fprintln(os.Stderr, "外部APIに投稿中...")
	if err := poster.PostToAPI(title, h.Options.Mode, generatedScript); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 外部APIへの投稿に失敗しました: %v\n", err)
	} else {
		fmt.Fprintln(os.Stderr, "外部APIへの投稿が完了しました。")
	}

	return nil
}
