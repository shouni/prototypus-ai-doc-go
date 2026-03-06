package prompt

import (
	_ "embed"
)

const (
	ModeDuet     = "duet"
	ModeSolo     = "solo"
	ModeDialogue = "dialogue"
)

var (
	//go:embed zundametan_duet.md
	duetPrompt string
	//go:embed zundamon_solo.md
	soloPrompt string
	//go:embed zundametan_dialogue.md
	dialoguePrompt string

	// modeTemplates はモードとテンプレート文字列を紐づけるマップです。
	modeTemplates = map[string]string{
		ModeDuet:     duetPrompt,
		ModeSolo:     soloPrompt,
		ModeDialogue: dialoguePrompt,
	}
)
