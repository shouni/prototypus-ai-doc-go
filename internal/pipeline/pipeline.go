package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"text/template"
	"time"

	"prototypus-ai-doc-go/internal/poster"
	"prototypus-ai-doc-go/internal/prompt"

	"github.com/shouni/go-ai-client/v2/pkg/ai/gemini"
	"github.com/shouni/go-utils/iohandler"
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
	Options                GenerateOptions
	Extractor              *extract.Extractor
	AiClient               *gemini.Client
	VoicevoxEngineExecutor voicevox.EngineExecutor
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
	slog.Info("処理開始",
		"mode", h.Options.Mode,
		"model", h.Options.AIModel,
		"input_size", len(inputContent))
	slog.Info("AIによるスクリプト生成を開始します...")

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
	slog.Info("AI スクリプト生成完了", "script", generatedScript)

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
	slog.Info("ファイルから読み込み中", "file", filePath)
	return iohandler.ReadInput(filePath)
}

// readFromURL はURLからコンテンツを取得します。
func (h *GenerateHandler) readFromURL(ctx context.Context) ([]byte, error) {
	slog.Info("URLからコンテンツを取得中", "url", h.Options.ScriptURL, "timeout", h.Options.HTTPTimeout.String())

	text, hasBodyFound, err := h.Extractor.FetchAndExtractText(h.Options.ScriptURL, ctx)
	if err != nil {
		return nil, fmt.Errorf("URLからのコンテンツ取得に失敗しました: %w", err)
	}
	if !hasBodyFound {
		slog.Info("記事本文が見つかりませんでした。タイトルのみで処理を続行します。", "url", h.Options.ScriptURL)
	}
	return []byte(text), nil
}

// readFromFile はファイルまたは標準入力からコンテンツを読み込みます。
func (h *GenerateHandler) readFromFile() ([]byte, error) {
	if h.Options.ScriptFile == "-" {
		slog.Info("標準入力 (stdin) から読み込み開始...")
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
	slog.Info("標準入力 (stdin) から読み込み開始...")
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

// handleVoicevoxOutput は VOICEVOX 処理を実行し、結果を出力します。
func (h *GenerateHandler) handleVoicevoxOutput(ctx context.Context, generatedScript string) error {
	if h.Options.VoicevoxOutput == "" {
		return nil
	}

	slog.InfoContext(ctx, "VOICEVOXエンジンに接続し、音声合成を開始します。", "output_file", h.Options.VoicevoxOutput)
	// 注入された Executor を直接実行
	err := h.VoicevoxEngineExecutor.Execute(ctx, generatedScript, h.Options.VoicevoxOutput, h.Options.VoicevoxFallbackTag)

	if err != nil {
		return fmt.Errorf("音声合成パイプラインの実行に失敗しました: %w", err)
	}
	slog.Info("VOICEVOXによる音声合成が完了し、ファイルに保存されました。", "output_file", h.Options.VoicevoxOutput)

	return nil
}

// --------------------------------------------------------------------------------
// ヘルパー関数 (出力処理)
// --------------------------------------------------------------------------------

// handleFinalOutput はスクリプトをファイルまたは標準出力に出力します。
func (h *GenerateHandler) handleFinalOutput(generatedScript string) error {
	return iohandler.WriteOutput(h.Options.OutputFile, []byte(generatedScript))
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
	slog.Info("外部APIに投稿中...")

	if err := poster.PostToAPI(title, h.Options.Mode, generatedScript); err != nil {
		slog.Warn("外部APIへの投稿に失敗しました。", "error", err)
	} else {
		slog.Info("外部APIへの投稿が完了しました。")
	}

	return nil
}
