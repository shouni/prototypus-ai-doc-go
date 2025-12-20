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
	"github.com/shouni/go-remote-io/pkg/remoteio"
	"github.com/shouni/go-web-exact/v2/pkg/extract"
)

// promptBuilderインターフェースをrunnerパッケージ内で定義する
type promptBuilder interface {
	Build(data prompt.TemplateData) (string, error)
}

// GenerateRunner は、ナレーションスクリプト生成を実行する責務を持つインターフェースです。
type GenerateRunner interface {
	Run(ctx context.Context) (string, error)
}

// DefaultGenerateRunner は generate コマンドの実行に必要な依存とオプションを保持します。
type DefaultGenerateRunner struct {
	options       config.GenerateOptions
	extractor     *extract.Extractor
	promptBuilder promptBuilder
	aiClient      *gemini.Client
	reader        remoteio.InputReader
}

// NewDefaultGenerateRunner は、依存関係を注入して DefaultGenerateRunner の新しいインスタンスを生成します。
func NewDefaultGenerateRunner(
	options config.GenerateOptions,
	extractor *extract.Extractor,
	promptBuilder promptBuilder,
	aiClient *gemini.Client,
	reader remoteio.InputReader,
) *DefaultGenerateRunner {
	return &DefaultGenerateRunner{
		options:       options,
		extractor:     extractor,
		promptBuilder: promptBuilder,
		aiClient:      aiClient,
		reader:        reader,
	}
}

// Run はナレーションスクリプト生成します。
func (gr *DefaultGenerateRunner) Run(ctx context.Context) (string, error) {
	inputContent, err := gr.readInputContent(ctx)
	if err != nil {
		return "", err
	}

	slog.Info("処理開始", "mode", gr.options.Mode, "model", gr.options.AIModel, "input_size", len(inputContent))
	slog.Info("AIによるスクリプト生成を開始します...")

	promptContent, err := gr.buildFullPrompt(string(inputContent))
	if err != nil {
		return "", err
	}

	generatedResponse, err := gr.aiClient.GenerateContent(ctx, promptContent, gr.options.AIModel)
	if err != nil {
		return "", fmt.Errorf("スクリプト生成に失敗しました: %w", err)
	}
	slog.Info("AI スクリプト生成完了", "script_length", len(generatedResponse.Text))

	return generatedResponse.Text, nil
}

// --------------------------------------------------------------------------------
// ヘルパー関数 (入力処理)
// --------------------------------------------------------------------------------

func (gr *DefaultGenerateRunner) readFromURL(ctx context.Context) ([]byte, error) {
	slog.Info("URLからコンテンツを取得中", "url", gr.options.ScriptURL, "timeout", gr.options.HTTPTimeout.String())

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
	var inputContent []byte
	var err error

	switch {
	case gr.options.ScriptURL != "":
		inputContent, err = gr.readFromURL(ctx)
	default:
		// ScriptFile が空の場合は標準入力として扱うように、
		// UniversalInputReader は空文字列や特定のパスを処理できる前提です。
		// もし stdin を明示的に扱いたい場合は path に "-" を渡す等のルールを運用します。
		path := gr.options.ScriptFile

		// クラウド/ローカル両対応の Reader を使用
		rc, openErr := gr.reader.Open(ctx, path)
		if openErr != nil {
			return nil, fmt.Errorf("入力ソースのオープンに失敗しました (%s): %w", path, openErr)
		}
		defer rc.Close()

		inputContent, err = io.ReadAll(rc)
	}

	if err != nil {
		if errors.Is(err, io.EOF) && len(inputContent) == 0 && gr.options.ScriptFile == "" {
			return nil, fmt.Errorf("標準入力が空です。文章を入力してください。")
		}
		return nil, fmt.Errorf("コンテンツの読み込み中にエラーが発生しました: %w", err)
	}

	trimmedContent := strings.TrimSpace(string(inputContent))
	if len(trimmedContent) < config.MinInputContentLength {
		return nil, fmt.Errorf("入力されたコンテンツが短すぎます (最低%dバイト必要です)。", config.MinInputContentLength)
	}

	return []byte(trimmedContent), nil
}

func (gr *DefaultGenerateRunner) buildFullPrompt(inputText string) (string, error) {
	data := prompt.TemplateData{InputText: inputText}
	fullPromptString, err := gr.promptBuilder.Build(data)
	if err != nil {
		return "", fmt.Errorf("プロンプトの構築に失敗しました: %w", err)
	}

	return fullPromptString, nil
}
