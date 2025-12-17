package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"prototypus-ai-doc-go/internal/config"
	"prototypus-ai-doc-go/internal/prompt"

	"github.com/shouni/go-ai-client/v2/pkg/ai/gemini"
	"github.com/shouni/go-utils/iohandler"
	"github.com/shouni/go-voicevox/pkg/voicevox"
	"github.com/shouni/go-web-exact/v2/pkg/extract"
)

// GenerateRunner は、ナレーションスクリプト生成を実行する責務を持つインターフェースです。
type GenerateRunner interface {
	Run(ctx context.Context) error
}

// DefaultGenerateRunner は generate コマンドの実行に必要な依存とオプションを保持します。
type DefaultGenerateRunner struct {
	options        config.GenerateOptions
	extractor      *extract.Extractor
	promptBuilder  prompt.PromptBuilder
	aiClient       *gemini.Client
	voicevoxEngine voicevox.EngineExecutor
}

// NewDefaultGenerateRunner は DefaultGenerateRunner のコンストラクタです。
// 通常、コンストラクタに context は不要なため削除していますが、初期化時に通信等が必要なら残してください。
func NewDefaultGenerateRunner(
	options config.GenerateOptions,
	extractor *extract.Extractor,
	promptBuilder prompt.PromptBuilder,
	aiClient *gemini.Client,
	voicevoxEngine voicevox.EngineExecutor,
) *DefaultGenerateRunner {
	return &DefaultGenerateRunner{
		options:        options,
		extractor:      extractor,
		promptBuilder:  promptBuilder,
		aiClient:       aiClient,
		voicevoxEngine: voicevoxEngine,
	}
}

// Run は実行します。
func (gr *DefaultGenerateRunner) Run(ctx context.Context) error {
	inputContent, err := gr.readInputContent(ctx)
	if err != nil {
		return err
	}

	// ログ出力
	slog.Info("処理開始", "mode", gr.options.Mode, "model", gr.options.AIModel, "input_size", len(inputContent))
	slog.Info("AIによるスクリプト生成を開始します...")

	// プロンプトの構築
	promptContent, err := gr.buildFullPrompt(string(inputContent))
	if err != nil {
		return err
	}

	// AIによるスクリプト生成
	generatedResponse, err := gr.aiClient.GenerateContent(ctx, promptContent, gr.options.AIModel)
	if err != nil {
		return fmt.Errorf("スクリプト生成に失敗しました: %w", err)
	}
	generatedScript := generatedResponse.Text
	slog.Info("AI スクリプト生成完了", "script_length", len(generatedScript))

	// VOICEVOX出力の処理
	if gr.options.VoicevoxOutput != "" {
		if err := gr.handleVoicevoxOutput(ctx, generatedScript); err != nil {
			return err
		}
		// VOICEVOX出力が成功した場合、ここで処理を終了 (早期リターン)
		return nil
	}

	// 通常のI/O出力
	return gr.handleFinalOutput(generatedScript)
}

// --------------------------------------------------------------------------------
// ヘルパー関数 (入力処理)
// --------------------------------------------------------------------------------

// readFromURL はURLからコンテンツを取得します。
func (gr *DefaultGenerateRunner) readFromURL(ctx context.Context) ([]byte, error) {
	slog.Info("URLからコンテンツを取得中", "url", gr.options.ScriptURL, "timeout")

	text, hasBodyFound, err := gr.extractor.FetchAndExtractText(ctx, gr.options.ScriptURL)
	if err != nil {
		return nil, fmt.Errorf("URLからのコンテンツ取得に失敗しました: %w", err)
	}
	if !hasBodyFound {
		slog.Info("記事本文が見つかりませんでした。タイトルのみで処理を続行します。", "url", gr.options.ScriptURL)
	}
	return []byte(text), nil
}

// readInputContent は入力ソースからコンテンツを読み込みます。
func (gr *DefaultGenerateRunner) readInputContent(ctx context.Context) ([]byte, error) {
	// 早期リターン条件チェックの強化: VOICEVOX出力時はOutputFileは不要
	if gr.options.VoicevoxOutput != "" && gr.options.OutputFile != "" {
		return nil, fmt.Errorf("voicevox出力(-v)とファイル出力(-o)は同時に指定できません。どちらか一方のみ指定してください")
	}

	var inputContent []byte
	var err error

	switch {
	case gr.options.ScriptURL != "":
		inputContent, err = gr.readFromURL(ctx)
	case gr.options.ScriptFile != "":
		inputContent, err = iohandler.ReadInput(gr.options.ScriptFile)
	default:
		inputContent, err = iohandler.ReadInput("")
	}

	if err != nil {
		// iohandler からの標準入力 EOF エラーをより親切なメッセージに変換
		if errors.Is(err, io.EOF) && len(inputContent) == 0 && gr.options.ScriptFile == "" {
			return nil, fmt.Errorf("標準入力が空です。文章を入力してください。")
		}
		return nil, err
	}

	trimmedContent := strings.TrimSpace(string(inputContent))
	if len(trimmedContent) < config.MinInputContentLength {
		return nil, fmt.Errorf("入力されたコンテンツが短すぎます (最低%dバイト必要です)。", config.MinInputContentLength)
	}

	// TrimSpace後のバイト配列を返す
	return []byte(trimmedContent), nil
}

// --------------------------------------------------------------------------------
// ヘルパー関数 (AI/VOICEVOX処理)
// --------------------------------------------------------------------------------

// buildFullPrompt はプロンプトテンプレートを構築し、入力内容を埋め込みます。
func (gr *DefaultGenerateRunner) buildFullPrompt(inputText string) (string, error) {
	data := prompt.TemplateData{InputText: inputText}
	fullPromptString, err := gr.promptBuilder.Build(data)
	if err != nil {
		return "", fmt.Errorf("プロンプトの構築に失敗しました: %w", err)
	}

	return fullPromptString, nil
}

// handleVoicevoxOutput は VOICEVOX 処理を実行し、結果を出力します。
func (gr *DefaultGenerateRunner) handleVoicevoxOutput(ctx context.Context, generatedScript string) error {
	slog.InfoContext(ctx, "VOICEVOXエンジンに接続し、音声合成を開始します。", "output_file", gr.options.VoicevoxOutput)

	err := gr.voicevoxEngine.Execute(ctx, generatedScript, gr.options.VoicevoxOutput)

	if err != nil {
		return fmt.Errorf("音声合成パイプラインの実行に失敗しました: %w", err)
	}
	slog.Info("VOICEVOXによる音声合成が完了し、ファイルに保存されました。", "output_file", gr.options.VoicevoxOutput)

	return nil
}

// --------------------------------------------------------------------------------
// ヘルパー関数 (出力処理)
// --------------------------------------------------------------------------------

// handleFinalOutput はスクリプトをファイルまたは標準出力に出力します。
func (gr *DefaultGenerateRunner) handleFinalOutput(generatedScript string) error {
	return iohandler.WriteOutputString(gr.options.OutputFile, generatedScript)
}
