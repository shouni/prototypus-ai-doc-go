package prompt

import (
	_ "embed"
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
		"duet":     duetPrompt,
		"solo":     soloPrompt,
		"dialogue": dialoguePrompt,
	}
)
