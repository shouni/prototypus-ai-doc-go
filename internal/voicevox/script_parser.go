package voicevox

import (
	"bytes"
	"regexp"
	"strings"
)

// ----------------------------------------------------------------------
// 演出用感情タグの定義 (変更なし)
// ----------------------------------------------------------------------
const emotionTagsPattern = `(解説|疑問|驚き|理解|落ち着き|納得|断定|呼びかけ|まとめ|通常|喜び|怒り|ノーマル|あまあま|ツンツン|セクシー|ヒソヒソ|ささやき)`

var (
	reScriptParse  = regexp.MustCompile(`^(\[.+?\])\s*(\[.+?\])\s*(.*)`)
	reEmotionParse = regexp.MustCompile(`\[` + emotionTagsPattern + `\]`)
	// 最大テキスト長をバイト数で設定（VOICEVOXの限界回避のため、日本語を考慮して厳しめに設定）
	maxSegmentByteLength = 1000
)

// ----------------------------------------------------------------------
// スクリプト解析ロジック
// ----------------------------------------------------------------------

// parseScript はスクリプトを話者・スタイルのタグが変わるか、最大文字数に達するまで結合します。
func parseScript(script string) []scriptSegment {
	lines := bytes.Split([]byte(script), []byte("\n"))
	var segments []scriptSegment

	var currentTag string
	var currentText strings.Builder

	// 結合されたセグメントを確定してリセットするヘルパー関数
	flushSegment := func() {
		if currentText.Len() > 0 && currentTag != "" {
			// 感情タグを除去し、スペースをトリム
			finalText := reEmotionParse.ReplaceAllString(currentText.String(), "")
			finalText = strings.TrimSpace(finalText)

			if finalText != "" {
				segments = append(segments, scriptSegment{
					SpeakerTag: currentTag,
					Text:       finalText,
				})
			}
		}
		// バッファをリセット
		currentTag = ""
		currentText.Reset()
	}

	for _, lineBytes := range lines {
		line := string(bytes.TrimSpace(lineBytes))
		if line == "" {
			continue
		}

		matches := reScriptParse.FindStringSubmatch(line)

		if len(matches) > 3 {
			// ★ タグ行の処理
			speakerTag := matches[1]
			vvStyleTag := matches[2]
			textPart := matches[3]

			newCombinedTag := speakerTag + vvStyleTag

			// 結合後の長さチェック
			// 既存のテキストに新しいテキストを追加した場合の長さ
			potentialLen := currentText.Len() + 1 + len(textPart)

			if currentTag == "" {
				// 最初の行の場合、バッファを開始
				currentTag = newCombinedTag
				currentText.WriteString(textPart)
			} else if newCombinedTag != currentTag || potentialLen > maxSegmentByteLength {
				// タグが変わった、または最大文字数を超えた場合
				flushSegment() // 古いセグメントを確定

				// 新しいセグメントを開始
				currentTag = newCombinedTag
				currentText.WriteString(textPart)
			} else {
				// タグが同じで、文字数制限内であれば結合を継続
				currentText.WriteString(" ") // 改行をスペースに変換
				currentText.WriteString(textPart)
			}

		} else if currentTag != "" {
			// ★ タグがない行（前のタグを引き継いで結合）

			// 結合後の長さチェック
			potentialLen := currentText.Len() + 1 + len(line)

			if potentialLen > maxSegmentByteLength {
				// 結合中に最大文字数を超えた場合、一旦セグメントを確定し、
				// 現在の行は新しいセグメントとして開始する (タグなしの行がセグメントの先頭になるのは稀だが、安全策)
				flushSegment()
				// NOTE: この行はタグがないため、新しいセグメントとして開始できない。ここでは単にスキップする。
			} else {
				// タグが同じであるとみなし、テキストを結合
				currentText.WriteString(" ")
				currentText.WriteString(line)
			}
		}
		// マッチしない（不正な形式の）行は無視される
	}

	// ループ終了後、バッファに残っている最後のセグメントを確定
	flushSegment()

	return segments
}
