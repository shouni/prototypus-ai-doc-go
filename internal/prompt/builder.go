package prompt

import (
	_ "embed"
	"fmt"
)

//go:embed zundametan_dialogue.md
var ZundaMetanDialoguePrompt string

//go:embed zundamon_solo.md
var ZundamonSoloPrompt string

// GetPromptByMode は、指定されたモードに対応するプロンプト文字列を返します。
func GetPromptByMode(mode string) (string, error) {
	switch mode {
	case "dialogue":
		return ZundaMetanDialoguePrompt, nil
	case "solo":
		return ZundamonSoloPrompt, nil
	default:
		return "", fmt.Errorf("サポートされていないモード: %s", mode)
	}
}
