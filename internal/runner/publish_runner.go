package runner

import (
	"context"
	"fmt"
	"log/slog"

	"prototypus-ai-doc-go/internal/config"

	"github.com/shouni/go-utils/iohandler"
	"github.com/shouni/go-voicevox/pkg/voicevox"
)

// PublisherRunner は、レビュー結果の公開処理を実行する責務を持つインターフェースです。
type PublisherRunner interface {
	Run(ctx context.Context, scriptContent string) error
}

// DefaultPublisherRunner は、レビュー結果の公開処理を実行する具象構造体です。
type DefaultPublisherRunner struct {
	options          config.GenerateOptions
	voicevoxExecutor voicevox.EngineExecutor
}

// NewDefaultPublisherRunner は DefaultPublisherRunner の新しいインスタンスを作成します。
// options は config.Config (または GenerateOptions) を想定しています。
func NewDefaultPublisherRunner(options config.GenerateOptions, voicevoxExecutor voicevox.EngineExecutor) *DefaultPublisherRunner {
	return &DefaultPublisherRunner{
		options:          options,
		voicevoxExecutor: voicevoxExecutor,
	}
}

// Run は公開処理のパイプライン全体を実行します。
func (pr *DefaultPublisherRunner) Run(ctx context.Context, scriptContent string) error {
	// VOICEVOX出力の処理
	if pr.options.VoicevoxOutput != "" {
		if err := pr.handleVoicevoxOutput(ctx, scriptContent); err != nil {
			return err
		}
		// VOICEVOX出力が成功した場合、ここで処理を終了 (早期リターン)
		return nil
	}

	// 通常のI/O出力
	return pr.handleFinalOutput(scriptContent)
}

// --------------------------------------------------------------------------------
// ヘルパー関数 (出力処理)
// --------------------------------------------------------------------------------

// handleVoicevoxOutput は VOICEVOX 処理を実行し、結果を出力します。
func (pr *DefaultPublisherRunner) handleVoicevoxOutput(ctx context.Context, generatedScript string) error {
	slog.InfoContext(ctx, "VOICEVOXエンジンに接続し、音声合成を開始します。", "output_file", pr.options.VoicevoxOutput)

	err := pr.voicevoxExecutor.Execute(ctx, generatedScript, pr.options.VoicevoxOutput)

	if err != nil {
		return fmt.Errorf("音声合成パイプラインの実行に失敗しました: %w", err)
	}
	slog.Info("VOICEVOXによる音声合成が完了し、ファイルに保存されました。", "output_file", pr.options.VoicevoxOutput)

	return nil
}

// handleFinalOutput はスクリプトをファイルまたは標準出力に出力します。
func (pr *DefaultPublisherRunner) handleFinalOutput(generatedScript string) error {
	return iohandler.WriteOutputString(pr.options.OutputFile, generatedScript)
}
