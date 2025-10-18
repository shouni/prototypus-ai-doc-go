package voicevox

import (
	"bytes"
	"log/slog" // ★ 追加: 警告ログ用
	"regexp"
	"strings"
)

// ----------------------------------------------------------------------
// 演出用感情タグの定義
// ----------------------------------------------------------------------

const emotionTagsPattern = `(解説|疑問|驚き|理解|落ち着き|納得|断定|呼びかけ|まとめ|通常|喜び|怒り|ノーマル|あまあま|ツンツン|セクシー|ヒソヒソ|ささやき)`

var (
	reScriptParse  = regexp.MustCompile(`^(\[.+?\])\s*(\[.+?\])\s*(.*)`)
	reEmotionParse = regexp.MustCompile(`\[` + emotionTagsPattern + `\]`)
	// 最大テキスト長をバイト数で設定（VOICEVOXの限界回避のため、日本語を考慮して厳しめに設定）
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
			potentialLen := currentText.Len() + 1 + len(textPart)

			if currentTag == "" {
				// 最初の行の場合、バッファを開始
				currentTag = newCombinedTag
				currentText.WriteString(textPart)
			} else if newCombinedTag != currentTag || potentialLen > maxSegmentByteLength {
				// タグが変わった、または最大文字数を超えた場合

				if potentialLen > maxSegmentByteLength {
					slog.Warn("セグメントの最大文字数を超過しました。現在のセグメントを強制的に確定し、超過行のテキストは次のセグメントに持ち越されます。",
						"segment_bytes", currentText.Len(),
						"max_bytes", maxSegmentByteLength,
						"tag", currentTag)
				}

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
				// ★ 修正: タグのない超過テキストの破棄を防止し、警告する
				slog.Warn("タグのないテキスト行が最大セグメント文字数を超過しました。テキストは破棄され、音声合成されません。",
					"tag", currentTag,
					"max_bytes", maxSegmentByteLength,
					"text_lost", line)
				// この行のテキストは無視し、次の行の処理へ進む

			} else {
				// タグが同じであるとみなし、テキストを結合
				currentText.WriteString(" ")
				currentText.WriteString(line)
			}
		}
	}

	// ループ終了後、バッファに残っている最後のセグメントを確定
	flushSegment()

	return segments
}
