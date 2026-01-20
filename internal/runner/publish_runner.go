package runner

import (
	"context"
	"fmt"
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
	options          config.GenerateOptions
	voicevoxExecutor voicevox.EngineExecutor
	writer           remoteio.OutputWriter
}

// NewDefaultPublisherRunner は DefaultPublisherRunner の新しいインスタンスを作成します。
func NewDefaultPublisherRunner(options config.GenerateOptions, voicevoxExecutor voicevox.EngineExecutor, writer remoteio.OutputWriter) *DefaultPublisherRunner {
	return &DefaultPublisherRunner{
		options:          options,
		voicevoxExecutor: voicevoxExecutor,
		writer:           writer,
	}
}

// Run は公開処理のパイプライン全体を実行します。
func (pr *DefaultPublisherRunner) Run(ctx context.Context, scriptContent string) error {
	if pr.options.VoicevoxOutput != "" {
		// 音声合成パイプラインの実行
		err := pr.voicevoxExecutor.Execute(ctx, scriptContent, pr.options.VoicevoxOutput)
		if err != nil {
			return fmt.Errorf("音声合成パイプラインの実行に失敗しました: %w", err)
		}

		// スクリプトのアップロード
		ext := filepath.Ext(pr.options.VoicevoxOutput)
		txtPath := strings.TrimSuffix(pr.options.VoicevoxOutput, ext) + ".txt"
		contentReader := strings.NewReader(scriptContent)

		if err := pr.writer.Write(ctx, txtPath, contentReader, "text/plain"); err != nil {
			return fmt.Errorf("スクリプトのアップロードに失敗しました (%s): %w", txtPath, err)
		}
	}

	return iohandler.WriteOutputString(pr.options.OutputFile, scriptContent)
}
