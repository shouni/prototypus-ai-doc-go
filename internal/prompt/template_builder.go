package prompt

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"prototypus-ai-doc-go/internal/domain"
)

// templateData はプロンプトテンプレートに渡すデータ構造です。
type templateData struct {
	InputText string
}

// templateBuilder は PromptBuilder インターフェースの具体的な実装です。
type templateBuilder struct {
	tmpl *template.Template
}

// NewBuilder は PromptBuilder インターフェースを実装する新しいインスタンスを初期化します。
// テンプレート文字列を受け取り、それをパースして PromptBuilder を返します。
func NewBuilder(templateStr string) (domain.PromptBuilder, error) {
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
	return &templateBuilder{tmpl: tmpl}, nil
}

// Build は TemplateData を埋め込み、プロンプト文字列を完成させます。
func (b *templateBuilder) Build(inputText string) (string, error) {
	data := templateData{InputText: inputText}

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
