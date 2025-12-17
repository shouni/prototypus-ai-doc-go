package config

import (
	"time"
)

// DefaultHTTPTimeout はHTTPリクエストのデフォルトタイムアウトを定義します。
// DefaultModel はデフォルトの Google Gemini モデル名（例: "gemini-2.5-flash"）を指定します。
// MinInputContentLength は入力されたコンテンツの最小バイト。
const (
	DefaultHTTPTimeout    = 30 * time.Second
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
