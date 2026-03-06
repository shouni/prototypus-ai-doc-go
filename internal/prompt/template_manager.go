package prompt

import (
	_ "embed"
)

// templateData はプロンプトテンプレートに渡すデータ構造です。
type templateData struct {
	InputText string
}

const (
	ModeDuet     = "duet"
	ModeSolo     = "solo"
	ModeDialogue = "dialogue"
)

var (
	// modeTemplates はモードとテンプレート文字列を紐づけるマップです。
	modeTemplates = map[string]string{
		ModeDuet:     duetPrompt,
		ModeSolo:     soloPrompt,
		ModeDialogue: dialoguePrompt,
	}
	//go:embed zundametan_duet.md
	duetPrompt string
	//go:embed zundamon_solo.md
	soloPrompt string
	//go:embed zundametan_dialogue.md
	dialoguePrompt string
)
