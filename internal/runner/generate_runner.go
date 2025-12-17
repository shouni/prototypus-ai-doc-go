package runner

import (
	"context"

	"prototypus-ai-doc-go/internal/config"
	"prototypus-ai-doc-go/internal/prompt"

	"github.com/shouni/go-ai-client/v2/pkg/ai/gemini"
	"github.com/shouni/go-voicevox/pkg/voicevox"
	"github.com/shouni/go-web-exact/v2/pkg/extract"
)

// GenerateRunner は、レビュー結果の公開処理を実行する責務を持つインターフェースです。
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

func (p *DefaultGenerateRunner) Run(ctx context.Context) error {
	// ここに実行ロジックを実装
	return nil
}
