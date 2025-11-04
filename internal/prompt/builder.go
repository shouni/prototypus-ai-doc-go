package prompt

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed zundametan_dialogue.md
var ZundaMetanDialoguePrompt string

//go:embed zundamon_solo.md
var ZundamonSoloPrompt string

//go:embed zundametan_duet.md
var ZundaMetanDuetPrompt string

// TemplateData はプロンプトテンプレートに渡すデータ構造です。
type TemplateData struct {
	InputText string
}

// ----------------------------------------------------------------
// ビルダー実装
// ----------------------------------------------------------------

// Builder はプロンプトの構成とテンプレート実行を管理します。
type Builder struct {
	tmpl *template.Template
}

// NewBuilder は Builder を初期化します。
func NewBuilder(templateStr string) (*Builder, error) {
	// テンプレート名は一意であれば何でも良い
	tmpl, err := template.New("prompt_template").Parse(templateStr)
	if err != nil {
		// エラーをラップして、呼び出し元に即座に返却
		return nil, fmt.Errorf("プロンプトテンプレートの解析に失敗しました (テンプレート先頭: %.50s...): %w", templateStr, err)
	}
	// テンプレートのパースが成功した場合のみ Builder を返す
	return &Builder{tmpl: tmpl}, nil
}

// buildPrompt は検証済みのデータを使用してプロンプトを生成します。
func (b *Builder) buildPrompt(data TemplateData) (string, error) {
	// 修正: NewBuilder でエラーチェックが完了しているため、b.err のチェックは不要

	var sb strings.Builder
	if err := b.tmpl.Execute(&sb, data); err != nil {
		// エラーにテンプレート名を含めて返却
		// b.tmpl は NewBuilder が成功した時点で有効なので、安全にアクセス可能
		return "", fmt.Errorf("%sプロンプトの実行に失敗しました: %w", b.tmpl.Name(), err)
	}

	return sb.String(), nil
}

// Build は TemplateData を埋め込み、プロンプト文字列を完成させます。
func (b *Builder) Build(data TemplateData) (string, error) {
	// 1. データ検証を Build メソッド内で直接行う
	if strings.TrimSpace(data.InputText) == "" {
		// エラーメッセージにテンプレート名を含めることで、デバッグ時の情報量を増やす
		return "", fmt.Errorf("%sプロンプト実行失敗: TemplateData.InputTextが空または空白のみです", b.tmpl.Name())
	}

	// 2. 検証済みのデータで buildPrompt を実行
	return b.buildPrompt(data)
}

// GetPromptByMode は、指定されたモードに対応するプロンプト文字列を返します。
func GetPromptByMode(mode string) (string, error) {
	switch mode {
	case "duet":
		return ZundaMetanDuetPrompt, nil
	case "solo":
		return ZundamonSoloPrompt, nil
	case "dialogue":
		return ZundaMetanDialoguePrompt, nil
	default:
		return "", fmt.Errorf("サポートされていないモード: %s. 'duet', 'solo', 'dialogue' のいずれかを指定してください", mode)
	}
}
