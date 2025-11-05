package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
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
// インターフェース定義 (リファクタリングのため追加)
// --------------------------------------------------------------------------------

// PromptBuilder は prompt.Builder の Build メソッドをラップするインターフェース
type PromptBuilder interface {
	Build(data prompt.TemplateData) (string, error)
}

// --------------------------------------------------------------------------------
// 構造体定義
// --------------------------------------------------------------------------------

// GenerateOptions はコマンドラインフラグを保持する構造体です。
type GenerateOptions struct {
	OutputFile     string
	Mode           string
	PostAPI        bool
	VoicevoxOutput string
	ScriptURL      string
	ScriptFile     string
	AIModel        string
	HTTPTimeout    time.Duration
}

// GenerateHandler は generate コマンドの実行に必要な依存とオプションを保持します。
type GenerateHandler struct {
	Options                GenerateOptions
	Extractor              *extract.Extractor
	PromptBuilder          PromptBuilder
	AiClient               *gemini.Client
	VoicevoxEngineExecutor voicevox.EngineExecutor // voicevox.Executor へのリネームを仮定しなかった場合
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
	// リトライ処理などは AiClient 内部に依存
	generatedResponse, err := h.AiClient.GenerateContent(ctx, promptContent, h.Options.AIModel)
	if err != nil {
		return fmt.Errorf("スクリプト生成に失敗しました: %w", err)
	}
	generatedScript := generatedResponse.Text

	slog.Info("AI スクリプト生成完了", "script_length", len(generatedScript))

	// 4. VOICEVOX出力の処理
	if h.Options.VoicevoxOutput != "" {
		if err := h.handleVoicevoxOutput(ctx, generatedScript); err != nil {
			return err
		}
		// VOICEVOX出力が成功した場合、ここで処理を終了 (早期リターン)
		return nil
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

// readFromURL はURLからコンテンツを取得します。
func (h *GenerateHandler) readFromURL(ctx context.Context) ([]byte, error) {
	slog.Info("URLからコンテンツを取得中", "url", h.Options.ScriptURL, "timeout", h.Options.HTTPTimeout.String())

	text, hasBodyFound, err := h.Extractor.FetchAndExtractText(ctx, h.Options.ScriptURL)
	if err != nil {
		return nil, fmt.Errorf("URLからのコンテンツ取得に失敗しました: %w", err)
	}
	if !hasBodyFound {
		slog.Info("記事本文が見つかりませんでした。タイトルのみで処理を続行します。", "url", h.Options.ScriptURL)
	}
	return []byte(text), nil
}

// readInputContent は入力ソースからコンテンツを読み込みます。
func (h *GenerateHandler) readInputContent(ctx context.Context) ([]byte, error) {
	// 早期リターン条件チェックの強化: VOICEVOX出力時はOutputFileは不要
	if h.Options.VoicevoxOutput != "" && h.Options.OutputFile != "" {
		return nil, fmt.Errorf("voicevox出力(-v)とファイル出力(-o)は同時に指定できません。どちらか一方のみ指定してください")
	}

	var inputContent []byte
	var err error

	switch {
	case h.Options.ScriptURL != "":
		inputContent, err = h.readFromURL(ctx)
	case h.Options.ScriptFile != "":
		// 修正: iohandler.ReadInput を活用し、ファイル名または "-" (stdin) を渡す
		inputContent, err = iohandler.ReadInput(h.Options.ScriptFile)
	default:
		// 修正: iohandler.ReadInput を活用し、ファイル名なし (= stdin) を渡す
		inputContent, err = iohandler.ReadInput("")
	}

	if err != nil {
		// iohandler からの標準入力 EOF エラーをより親切なメッセージに変換
		if errors.Is(err, io.EOF) && len(inputContent) == 0 && h.Options.ScriptFile == "" {
			return nil, fmt.Errorf("標準入力が空です。文章を入力してください。")
		}
		return nil, err
	}

	// リファクタリング: 文字列化してTrimSpaceしてから len をチェック
	trimmedContent := strings.TrimSpace(string(inputContent))
	if len(trimmedContent) < MinInputContentLength {
		return nil, fmt.Errorf("入力されたコンテンツが短すぎます (最低%dバイト必要です)。", MinInputContentLength)
	}

	// TrimSpace後のバイト配列を返す
	return []byte(trimmedContent), nil
}

// --------------------------------------------------------------------------------
// ヘルパー関数 (AI/VOICEVOX処理)
// --------------------------------------------------------------------------------

// buildFullPrompt はプロンプトテンプレートを構築し、入力内容を埋め込みます。
func (h *GenerateHandler) buildFullPrompt(inputText string) (string, error) {
	// リファクタリング: InputText のTrimSpaceは readInputContent で行ったため、ここでは行わない
	data := prompt.TemplateData{InputText: inputText}
	fullPromptString, err := h.PromptBuilder.Build(data)
	if err != nil {
		return "", fmt.Errorf("プロンプトの構築に失敗しました: %w", err)
	}

	return fullPromptString, nil
}

// handleVoicevoxOutput は VOICEVOX 処理を実行し、結果を出力します。
func (h *GenerateHandler) handleVoicevoxOutput(ctx context.Context, generatedScript string) error {
	slog.InfoContext(ctx, "VOICEVOXエンジンに接続し、音声合成を開始します。", "output_file", h.Options.VoicevoxOutput)

	// 注入された Executor を直接実行
	err := h.VoicevoxEngineExecutor.Execute(ctx, generatedScript, h.Options.VoicevoxOutput)

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
	return iohandler.WriteOutputString(h.Options.OutputFile, generatedScript)
}

// generatePostTitle は API 投稿用のタイトルを生成します。
// (ロジックの変更なし - そのまま維持)
func (h *GenerateHandler) generatePostTitle(inputContent []byte) string {
	if h.Options.OutputFile != "" {
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
// (ロジックの変更なし - そのまま維持)
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
