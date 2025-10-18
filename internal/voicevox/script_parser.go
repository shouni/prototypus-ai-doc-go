package voicevox

import (
	"bytes"
	"log/slog"
	"regexp"
	"strings"
)

// ----------------------------------------------------------------------
// 定数・変数
// ----------------------------------------------------------------------

const emotionTagsPattern = `(解説|疑問|驚き|理解|落ち着き|納得|断定|呼びかけ|まとめ|通常|喜び|怒り|ノーマル|あまあま|ツンツン|セクシー|ヒソヒソ|ささやき)`

var (
	reScriptParse  = regexp.MustCompile(`^(\[.+?\])\s*(\[.+?\])\s*(.*)`)
	reEmotionParse = regexp.MustCompile(`\[` + emotionTagsPattern + `\]`)
	// ★ 修正: バイト数制限であることを明確化。VOICEVOXエンジンが安全に処理できる最大文字数の目安 (約300文字の日本語) に相当するバイト数。
	maxSegmentByteLength = 1000
)

// scriptSegment は engine.go で定義されたものを利用します。（ここでは再定義を省略）
// type scriptSegment struct { ... }

// ----------------------------------------------------------------------
// スクリプト解析ロジック
// ----------------------------------------------------------------------

// parseScript はスクリプトを話者・スタイルのタグが変わるか、最大文字数に達するまで結合します。
func parseScript(script string) []scriptSegment {
	lines := bytes.Split([]byte(script), []byte("\n"))
	var segments []scriptSegment

	var currentTag string
	var currentText strings.Builder
	var textBuffer string // ★ 追加: タグがない行の超過テキストを一時保持するバッファ

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
		// タグはリセットするが、textBufferは保持されたまま
		currentTag = ""
		currentText.Reset()
	}

	// ★ 修正: 行処理ループ
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

			// 処理対象テキスト: タグのない超過テキストバッファ + 今回のテキスト
			fullTextToAppend := textBuffer + " " + textPart
			textBuffer = "" // textBufferをクリア

			// 結合後の長さチェック
			potentialLen := currentText.Len() + 1 + len(fullTextToAppend)

			if currentTag == "" {
				// 最初の行（またはflushSegment直後）の場合、バッファを開始
				currentTag = newCombinedTag
				currentText.WriteString(fullTextToAppend)
			} else if newCombinedTag != currentTag || potentialLen > maxSegmentByteLength {
				// タグが変わった、または最大文字数を超えた場合

				if potentialLen > maxSegmentByteLength {
					slog.Warn("セグメントの最大文字数を超過しました。現在のセグメントを強制的に確定し、超過行は新しいセグメントとして開始されます。",
						"segment_bytes", currentText.Len(),
						"max_bytes", maxSegmentByteLength,
						"tag", currentTag)
				}

				flushSegment() // 古いセグメントを確定

				// 新しいセグメントを開始
				currentTag = newCombinedTag
				currentText.WriteString(fullTextToAppend)
			} else {
				// タグが同じで、文字数制限内であれば結合を継続
				currentText.WriteString(" ")
				currentText.WriteString(fullTextToAppend)
			}

		} else if currentTag != "" {
			// ★ タグがない行（前のタグを引き継いで結合）

			// 結合後の長さチェック
			potentialLen := currentText.Len() + 1 + len(line)

			if potentialLen > maxSegmentByteLength {
				// ★ 修正: 超過テキストを破棄せず、一時バッファに保持して次のタグ行で処理する

				// 現在のセグメントを確定 (文字数制限内でできる限り合成)
				flushSegment()

				slog.Warn("タグのないテキスト行が最大セグメント文字数を超過したため、テキストを一時バッファに保持し、次のタグ付きセグメントに結合します。",
					"tag", currentTag,
					"max_bytes", maxSegmentByteLength,
					"text_overflow", line)

				// 超過したタグなしテキストをバッファに保持
				textBuffer = line

			} else {
				// タグが同じであるとみなし、テキストを結合
				currentText.WriteString(" ")
				currentText.WriteString(line)
			}
		}
	}

	// ループ終了後、バッファに残っている最後のセグメントを確定
	flushSegment()

	// 最後に textBuffer に何かが残っている場合は、タグなしテキストが無視された可能性があるため、致命的な警告
	if textBuffer != "" {
		slog.Error("スクリプトの最後にタグのない超過テキストが残されました。このテキストは合成されません。", "lost_text", textBuffer)
	}

	return segments
}
