package prompt

import (
	_ "embed"
)

const (
	modeDuet     = "duet"
	modeSolo     = "solo"
	modeDialogue = "dialogue"
)

var (
	//go:embed zundametan_duet.md
	duetPrompt string
	//go:embed zundamon_solo.md
	soloPrompt string
	//go:embed zundametan_dialogue.md
	dialoguePrompt string
)

// modeTemplates はモードとテンプレート文字列を紐づけるマップです。
var modeTemplates = map[string]string{
	modeDuet:     duetPrompt,
	modeSolo:     soloPrompt,
	modeDialogue: dialoguePrompt,
}
