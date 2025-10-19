package voicevox

import (
	"log/slog"
	"regexp"
	"strings"
	"unicode/utf8"
)

const emotionTagsPattern = `(解説|疑問|驚き|理解|落ち着き|納得|断定|呼びかけ|まとめ|通常|喜び|怒り|ノーマル|あまあま|ツンツン|セクシー|ヒソヒソ|ささやき)`

var (
	reScriptParse  = regexp.MustCompile(`^(\[.+?\])\s*(\[.+?\])\s*(.*)`)
	reEmotionParse = regexp.MustCompile(`\[` + emotionTagsPattern + `\]`)
	// 最大テキスト長（文字数）。VOICEVOXが安全に処理できる最大文字数の目安として250文字に設定。
	maxSegmentCharLength = 250
)

// scriptParser はスクリプトの解析状態を管理し、セグメント化を実行します。
type scriptParser struct {
	segments    []scriptSegment
	currentTag  string
	currentText *strings.Builder
	textBuffer  string
	fallbackTag string
}

// newScriptParser は新しい scriptParser インスタンスを作成します。
func newScriptParser(fallbackTag string) *scriptParser {
	return &scriptParser{
		currentText: &strings.Builder{},
		fallbackTag: fallbackTag,
	}
}

// parse はスクリプト文字列を解析し、scriptSegment のスライスを返します。
func (p *scriptParser) parse(script string) []scriptSegment {
	lines := strings.Split(script, "\n")

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}
		p.processLine(trimmedLine)
	}

	p.finishParsing()
	return p.segments
}

// processLine はスクリプトの1行を処理します。
// line は parse() で既に TrimSpace されています。
func (p *scriptParser) processLine(line string) {
	// 修正1: 冗長な TrimSpace を削除し、lineをそのまま使用
	if line == "" {
		return
	}

	textToProcess := line
	if p.textBuffer != "" {
		textToProcess = p.textBuffer + " " + line
		p.textBuffer = "" // バッファをクリア
	}

	matches := reScriptParse.FindStringSubmatch(textToProcess)
	if len(matches) > 3 {
		speakerTag := matches[1]
		vvStyleTag := matches[2]
		textPart := matches[3]
		newCombinedTag := speakerTag + vvStyleTag
		p.processTaggedLine(newCombinedTag, textPart)
	} else {
		p.processUntaggedLine(textToProcess)
	}
}

// processTaggedLine はタグ付きの行を処理します。
func (p *scriptParser) processTaggedLine(tag, text string) {
	// タグが変更された場合、現在のセグメントを確定します。
	if p.currentTag != "" && tag != p.currentTag {
		p.flushCurrentSegment()
	}

	p.currentTag = tag
	p.appendAndSplitText(text)
}

// processUntaggedLine はタグのない行を処理します。
func (p *scriptParser) processUntaggedLine(text string) {
	if p.currentTag != "" {
		// 既存のセグメントにテキストを追記します。
		p.appendAndSplitText(text)
	} else {
		// 対応するタグがないため、テキストを一時バッファに保存します。
		p.textBuffer = text
		slog.Warn("タグのないテキスト行が検出されました。次のタグ付きセグメントに結合されます。", "text", text)
	}
}

// appendAndSplitText はテキストを現在のセグメントに追記し、必要に応じて分割します。
func (p *scriptParser) appendAndSplitText(text string) {
	textToAppend := text
	for textToAppend != "" {
		// 修正2: 句読点優先の分割ロジックを使用
		partToAdd, remainder := p.splitTextByPunctuation(textToAppend)

		if partToAdd != "" {
			if p.currentText.Len() > 0 {
				p.currentText.WriteString(" ")
			}
			p.currentText.WriteString(partToAdd)
		}

		if remainder != "" {
			slog.Warn("テキストが最大文字数を超過したため、セグメントを強制的に確定し、残りのテキストを分割します。",
				"max_chars", maxSegmentCharLength, "tag", p.currentTag)
			p.flushCurrentSegment()
			textToAppend = remainder
		} else {
			textToAppend = ""
		}
	}
}

// splitTextByPunctuation は、現在のセグメントの文字数制限と句読点に基づき、追記するテキストを分割します。
// 句読点（。、！？）を見つけ、その直後で分割することを優先します。
func (p *scriptParser) splitTextByPunctuation(text string) (partToAdd string, remainder string) {
	currentRuneCount := utf8.RuneCountInString(p.currentText.String())

	// 現在のセグメントが空でなければスペース1つ分加算
	space := 0
	if currentRuneCount > 0 {
		space = 1
	}

	// 1. テキスト全体を追加しても最大長を超えない場合
	if currentRuneCount+space+utf8.RuneCountInString(text) <= maxSegmentCharLength {
		return text, ""
	}

	// 2. 最大長を超過するため、分割位置を探す

	// 追加可能な最大文字数（スペース分を考慮）
	maxCapacity := maxSegmentCharLength - currentRuneCount - space

	// もし現在のセグメントが既に容量オーバーなら、partToAddは空で、text全体をremainderとして返し、
	// 呼び出し元で flushCurrentSegment() を促す。
	if maxCapacity <= 0 {
		return "", text
	}

	runes := []rune(text)

	// 許容量内で句読点を優先する分割位置を探す
	bestSplitIndex := -1
	// i+1 が現在のセグメントに追加されるルーン数
	for i := 0; i < len(runes); i++ {
		// 現在のセグメント長 + スペース + (i+1)文字が maxSegmentCharLength を超える場合、
		// i-1までが許容量内のギリギリの分割点となる
		if currentRuneCount+space+(i+1) > maxSegmentCharLength {
			break // 許容量を超えたのでループを抜ける
		}

		// 許容量内の位置で、句読点の直後を分割点として記録
		r := runes[i]
		if r == '。' || r == '、' || r == '！' || r == '？' {
			bestSplitIndex = i + 1 // 句読点を含めた直後で分割
		}
	}

	// 3. 分割の決定

	if bestSplitIndex > 0 {
		// 句読点が見つかった場合、その位置で分割
		partToAdd = string(runes[:bestSplitIndex])
		remainder = string(runes[bestSplitIndex:])
		return partToAdd, remainder
	}

	// 句読点が見つからなかった、かつ最大長を超える場合 (強制分割)
	if maxCapacity > 0 && maxCapacity < len(runes) {
		// 許容量内で強制的に分割
		partToAdd = string(runes[:maxCapacity])
		remainder = string(runes[maxCapacity:])
		return partToAdd, remainder
	}

	// ここに来ることは稀だが、text自体が短い場合のフォールバック
	return text, ""
}

// flushCurrentSegment は現在のテキストバッファを新しいセグメントとして確定し、バッファをリセットします。
func (p *scriptParser) flushCurrentSegment() {
	if p.currentText.Len() > 0 && p.currentTag != "" {
		p.addSegment(p.currentTag, p.currentText.String())
	}
	p.currentText.Reset()
}

// addSegment は整形後のテキストからセグメントを作成し、リストに追加します。
func (p *scriptParser) addSegment(tag string, text string) {
	finalText := reEmotionParse.ReplaceAllString(text, "")
	finalText = strings.TrimSpace(finalText)
	if finalText != "" {
		p.segments = append(p.segments, scriptSegment{
			SpeakerTag: tag,
			Text:       finalText,
		})
	}
}

// finishParsing は解析終了時に残っているバッファを処理します。
func (p *scriptParser) finishParsing() {
	p.flushCurrentSegment()

	if p.textBuffer != "" {
		if len(p.segments) > 0 {
			// 既存のセグメントが存在する場合、最後のタグを流用
			lastTag := p.segments[len(p.segments)-1].SpeakerTag
			slog.Warn("スクリプトの最後にタグのないテキストが残りました。最後のタグを流用して最終セグメントとして合成します。",
				"lost_text", p.textBuffer, "used_tag", lastTag)
			p.addSegment(lastTag, p.textBuffer)
		} else {
			// 既存のセグメントが一つもない場合 (タグなしテキストのみのスクリプト)
			slog.Warn("スクリプトにタグ付きセグメントがありませんでした。デフォルトタグを使用してテキスト全体を合成します。",
				"text_content", p.textBuffer, "default_tag", p.fallbackTag)
			if p.fallbackTag != "" {
				p.addSegment(p.fallbackTag, p.textBuffer)
			} else {
				slog.Error("スクリプトに有効なタグがなく、フォールバックタグも設定されていません。テキストは合成されません。", "lost_text", p.textBuffer)
			}
		}
	}
}

// parseScript はスクリプトを話者・スタイルのタグが変わるか、最大文字数に達するまで結合します。
func parseScript(script string, fallbackTag string) []scriptSegment {
	parser := newScriptParser(fallbackTag)
	return parser.parse(script)
}
