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
// 修正: フィールド名を Text から InputText に変更し、テンプレートファイルと一致させる
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

// buildPrompt はテンプレートデータを検証し、プロンプトを生成します。
func (b *Builder) buildPrompt(data interface{}, emptyCheckFunc func(data interface{}) error) (string, error) {
	// 1. テンプレート初期化時のエラーをチェック
	if b.err != nil {
		// b.tmpl.Name() が使えない可能性があるため、固定メッセージでエラーを出力
		return "", fmt.Errorf("プロンプトテンプレートの初期化に失敗しています: %w", b.err)
	}

	// 2. データ固有の空チェックを実行
	if err := emptyCheckFunc(data); err != nil {
		// emptyCheckFuncが具体的なフィールド名を含むエラーを返すため、それをそのまま利用
		return "", fmt.Errorf("プロンプト実行失敗: %w", err)
	}

	// 3. テンプレート実行 (strings.Builderを使用)
	var sb strings.Builder
	if err := b.tmpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("%sプロンプトの実行に失敗しました: %w", b.tmpl.Name(), err)
	}

	return sb.String(), nil
}

// Build は TemplateData を埋め込み、プロンプト文字列を完成させます。
func (b *Builder) Build(data TemplateData) (string, error) {
	return b.buildPrompt(data, func(d interface{}) error {
		if strings.TrimSpace(d.(TemplateData).InputText) == "" {
			// エラーメッセージも新しいフィールド名に合わせて修正
			return fmt.Errorf("TemplateData.InputTextが空または空白のみです")
		}
		return nil
	})
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
