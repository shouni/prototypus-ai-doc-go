package prompt

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// ----------------------------------------------------------------
// 抽象化とデータ構造
// ----------------------------------------------------------------

// TemplateData はプロンプトテンプレートに渡すデータ構造です。
type TemplateData struct {
	InputText string
}

// PromptBuilder は、テンプレートデータから最終的なプロンプト文字列を生成する責務を定義します。
// これにより、具体的な実装（text/templateなど）から利用側を分離できます。
type PromptBuilder interface {
	Build(data TemplateData) (string, error)
}

// ----------------------------------------------------------------
// ビルダー実装
// ----------------------------------------------------------------

// textTemplateBuilder は PromptBuilder インターフェースの具体的な実装です。
type textTemplateBuilder struct {
	tmpl *template.Template
}

// NewBuilder は PromptBuilder インターフェースを実装する新しいインスタンスを初期化します。
// テンプレート文字列を受け取り、それをパースして PromptBuilder を返します。
func NewBuilder(templateStr string) (PromptBuilder, error) {
	if strings.TrimSpace(templateStr) == "" {
		return nil, fmt.Errorf("プロンプトテンプレートの内容が空です")
	}

	// テンプレート名は一意であれば何でも良い。ここでは定数を使用。
	const templateName = "prompt_builder"

	// パースの失敗時もテンプレートの先頭を表示し、デバッグ情報を提供する。
	tmpl, err := template.New(templateName).Parse(templateStr)
	if err != nil {
		// エラーをラップ。templateStrの長さを考慮し、短く表示。
		snippet := templateStr
		if len(snippet) > 50 {
			snippet = snippet[:50] + "..."
		}
		return nil, fmt.Errorf("プロンプトテンプレートの解析に失敗しました (テンプレート先頭: %s): %w", snippet, err)
	}

	// インターフェース型として具体的な実装を返す
	return &textTemplateBuilder{tmpl: tmpl}, nil
}

// Build は TemplateData を埋め込み、プロンプト文字列を完成させます。
func (b *textTemplateBuilder) Build(data TemplateData) (string, error) {
	// 1. データ検証
	if strings.TrimSpace(data.InputText) == "" {
		// エラーメッセージにテンプレート名を含める (tmpl.Name()を使用)
		return "", fmt.Errorf("プロンプト実行失敗: TemplateData.InputTextが空または空白のみです (テンプレート: %s)", b.tmpl.Name())
	}

	// 2. テンプレート実行 (buildPromptのロジックを統合)
	var buf bytes.Buffer
	if err := b.tmpl.Execute(&buf, data); err != nil {
		// エラーにテンプレート名を含める
		return "", fmt.Errorf("プロンプトテンプレート '%s' の実行に失敗しました: %w", b.tmpl.Name(), err)
	}

	return buf.String(), nil
}
