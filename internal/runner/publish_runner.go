package runner

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"prototypus-ai-doc-go/internal/config"
	"strings"

	"github.com/shouni/go-remote-io/pkg/remoteio"
	"github.com/shouni/go-utils/iohandler"
	"github.com/shouni/go-voicevox/pkg/voicevox"
)

// PublisherRunner は、生成されたスクリプトの公開処理を実行する責務を持つインターフェースです。
type PublisherRunner interface {
	Run(ctx context.Context, scriptContent string) error
}

// DefaultPublisherRunner は、スクリプトの公開処理を実行する具象構造体です。
type DefaultPublisherRunner struct {
	options          *config.GenerateOptions
	voicevoxExecutor voicevox.EngineExecutor
	writer           remoteio.OutputWriter
}

// NewDefaultPublisherRunner は DefaultPublisherRunner の新しいインスタンスを作成します。
func NewDefaultPublisherRunner(options *config.GenerateOptions, voicevoxExecutor voicevox.EngineExecutor, writer remoteio.OutputWriter) *DefaultPublisherRunner {
	return &DefaultPublisherRunner{
		options:          options,
		voicevoxExecutor: voicevoxExecutor,
		writer:           writer,
	}
}

// Run は公開処理のパイプライン全体を実行します。
func (pr *DefaultPublisherRunner) Run(ctx context.Context, scriptContent string) error {
	if pr.options.VoicevoxOutput != "" {
		return pr.publishAudioAndScript(ctx, scriptContent)
	}

	return iohandler.WriteOutputString(pr.options.OutputFile, scriptContent)
}

// publishAudioAndScript は音声合成とスクリプトのアップロードを実行します。
func (pr *DefaultPublisherRunner) publishAudioAndScript(ctx context.Context, scriptContent string) error {
	// 音声合成
	slog.InfoContext(ctx, "VOICEVOXによる音声合成を開始します。", "output_path", pr.options.VoicevoxOutput)
	if err := pr.voicevoxExecutor.Execute(ctx, scriptContent, pr.options.VoicevoxOutput); err != nil {
		return fmt.Errorf("音声合成パイプラインの実行に失敗しました (%s): %w", pr.options.VoicevoxOutput, err)
	}
	slog.InfoContext(ctx, "音声合成が完了しました。", "output_path", pr.options.VoicevoxOutput)

	// スクリプトのアップロード
	ext := filepath.Ext(pr.options.VoicevoxOutput)
	txtPath := strings.TrimSuffix(pr.options.VoicevoxOutput, ext) + ".txt"
	contentReader := strings.NewReader(scriptContent)

	slog.InfoContext(ctx, "スクリプトのアップロードを開始します。", "upload_path", txtPath)
	if err := pr.writer.Write(ctx, txtPath, contentReader, "text/plain; charset=utf-8"); err != nil {
		return fmt.Errorf("スクリプトのアップロードに失敗しました (%s): %w", txtPath, err)
	}
	slog.InfoContext(ctx, "スクリプトのアップロードが完了しました。", "upload_path", txtPath)

	return nil
}
