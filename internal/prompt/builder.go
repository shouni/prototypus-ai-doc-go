package prompt

import (
	_ "embed"
	"fmt"
)

//go:embed zundametan_dialogue.md
var ZundaMetanDialoguePrompt string

//go:embed zundamon_solo.md
var ZundamonSoloPrompt string

//go:embed zundametan_duet.md
var ZundaMetanDuetPrompt string

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
		return "", fmt.Errorf("サポートされていないモード: %s. 'duet', 'solo', 'dialogue'のいずれかを指定してください", mode)
	}
}
