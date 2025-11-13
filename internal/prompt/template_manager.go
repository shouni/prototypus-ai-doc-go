package prompt

import (
	_ "embed"
	"fmt"
)

// --- モード定義 (定数) ---

const (
	ModeDuet     = "duet"
	ModeSolo     = "solo"
	ModeDialogue = "dialogue"
)

// --- テンプレートのリソース定義 (go:embed) ---

//go:embed zundametan_dialogue.md
var ZundaMetanDialoguePrompt string

//go:embed zundamon_solo.md
var ZundamonSoloPrompt string

//go:embed zundametan_duet.md
var ZundaMetanDuetPrompt string

// modeTemplates はモードとテンプレート文字列を紐づけるマップです。
var modeTemplates = map[string]string{
	ModeDuet:     ZundaMetanDuetPrompt,
	ModeSolo:     ZundamonSoloPrompt,
	ModeDialogue: ZundaMetanDialoguePrompt,
}

// GetPromptByMode は、指定されたモードに対応するプロンプト文字列を返します。
// 内部でテンプレートの内容が空でないかのチェックも行います。
func GetPromptByMode(mode string) (string, error) {
	content, ok := modeTemplates[mode]
	if !ok {
		// サポートされていないモードの場合
		return "", fmt.Errorf("サポートされていないモード: '%s'。'%s', '%s', '%s' のいずれかを指定してください",
			mode, ModeDuet, ModeSolo, ModeDialogue)
	}

	// テンプレートの内容が空でないか（go:embedが失敗していないか）のチェック
	if content == "" {
		// これは通常、go:embed がファイルを見つけられなかった場合に発生します。
		return "", fmt.Errorf("モード '%s' に対応するプロンプトテンプレートの内容が空です。embed設定を確認してください", mode)
	}

	return content, nil
}
