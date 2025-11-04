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
// テンプレートとの一貫性を保つため InputText を使用
type TemplateData struct {
	InputText string
}

// ----------------------------------------------------------------
// ビルダー実装
// ----------------------------------------------------------------

// Builder はプロンプトの構成とテンプレート実行を管理します。
type Builder struct {
	tmpl *template.Template
	err  error // テンプレートのパースエラーを保持
}

// NewBuilder は Builder を初期化します。
// templateStr: 実行に使用するテンプレート文字列。
func NewBuilder(templateStr string) *Builder {
	// テンプレート名は一意であれば何でも良い
	tmpl, err := template.New("prompt_template").Parse(templateStr)
	return &Builder{tmpl: tmpl, err: err}
}

// buildPrompt は検証済みのデータを使用してプロンプトを生成します。
// データ検証を Build メソッドに移し、buildPrompt は実行に専念させます。
func (b *Builder) buildPrompt(data interface{}) (string, error) {
	// 1. テンプレート初期化時のエラーをチェック
	if b.err != nil {
		// 元のエラーをラップして返却
		return "", fmt.Errorf("プロンプトテンプレートの初期化に失敗しています: %w", b.err)
	}

	// 2. テンプレート実行 (strings.Builderを使用)
	var sb strings.Builder
	if err := b.tmpl.Execute(&sb, data); err != nil {
		// エラーにテンプレート名を含めて返却
		return "", fmt.Errorf("%sプロンプトの実行に失敗しました: %w", b.tmpl.Name(), err)
	}

	return sb.String(), nil
}

// Build は TemplateData を埋め込み、プロンプト文字列を完成させます。
func (b *Builder) Build(data TemplateData) (string, error) {
	// 1. データ検証を Build メソッド内で直接行う
	if strings.TrimSpace(data.InputText) == "" {
		// エラーを返す
		return "", fmt.Errorf("プロンプト実行失敗: TemplateData.InputTextが空または空白のみです")
	}

	// 2. 検証済みのデータで buildPrompt を実行
	// buildPrompt は引数を一つに削減されたため、呼び出しも簡潔に
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
