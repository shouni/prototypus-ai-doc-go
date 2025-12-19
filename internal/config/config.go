package config

import (
	"errors"
	"time"

	"github.com/shouni/go-http-kit/pkg/httpkit"
)

// DefaultHTTPTimeout はHTTPリクエストのデフォルトタイムアウトを定義します。
// DefaultModel はデフォルトの Google Gemini モデル名（例: "gemini-2.5-flash"）を指定します。
// MinInputContentLength は入力されたコンテンツの最小バイト。
const (
	DefaultHTTPTimeout    = 60 * time.Second
	DefaultModel          = "gemini-2.5-flash"
	MinInputContentLength = 10
)

// GenerateOptions はコマンドラインフラグを保持する構造体です。
type GenerateOptions struct {
	OutputFile     string
	Mode           string
	VoicevoxOutput string
	ScriptURL      string
	ScriptFile     string
	AIModel        string
	HTTPTimeout    time.Duration
}

// AppContext は実行時の依存関係を保持するコンテナ
type AppContext struct {
	Options    GenerateOptions
	HTTPClient httpkit.ClientInterface
}

// NewAppContext は、与えられたGenerateOptionsからアプリケーションの実行に必要な
// 依存関係（HTTPクライアントなど）を初期化し、AppContextを生成します。
// この関数は、アプリケーションのライフサイクルで一度だけ呼び出されることを想定しています。
func NewAppContext(opts GenerateOptions) AppContext {
	timeout := opts.HTTPTimeout
	if timeout == 0 {
		timeout = DefaultHTTPTimeout
	}
	return AppContext{
		Options:    opts,
		HTTPClient: httpkit.New(timeout, httpkit.WithMaxRetries(3)),
	}
}

// Validate は、AppContextの必須フィールドが正しく初期化されていることを検証します。
// アプリケーションの実行前に呼び出されることを想定しています。
func (ac AppContext) Validate() error {
	if ac.HTTPClient == nil {
		return errors.New("HTTPClientが初期化されていません")
	}
	// 他の必須フィールドの検証もここに追加可能
	return nil
}
