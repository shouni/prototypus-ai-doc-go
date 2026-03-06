package prompt

import (
	"fmt"
	"strings"
	"text/template"

	"prototypus-ai-doc-go/internal/domain"
)

// templateBuilder は PromptBuilder インターフェースの具体的な実装です。
type templateBuilder struct {
	templates map[string]*template.Template
}

// NewBuilder は テンプレート文字列を受け取り、それをパースして templateBuilder を返します。
func NewBuilder() (domain.PromptBuilder, error) {
	parsedTemplates := make(map[string]*template.Template)
	for mode, content := range modeTemplates {
		if content == "" {
			return nil, fmt.Errorf("プロンプトテンプレート '%s' (go:embed) の読み込みに失敗しました: 内容が空です", mode)
		}

		tmpl, err := template.New(mode).Parse(content)
		if err != nil {
			return nil, fmt.Errorf("プロンプト '%s' の解析に失敗: %w", mode, err)
		}
		parsedTemplates[mode] = tmpl
	}

	return &templateBuilder{
		templates: parsedTemplates,
	}, nil
}

// Build は、要求されたモードに応じて適切なテンプレートを実行します。
func (b *templateBuilder) Build(mode string, inputText string) (string, error) {
	tmpl, ok := b.templates[mode]
	if !ok {
		return "", fmt.Errorf("不明なモードです: '%s'", mode)
	}

	var sb strings.Builder
	data := templateData{InputText: inputText}
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("プロンプトテンプレートの実行に失敗しました: %w", err)
	}

	return sb.String(), nil
}
