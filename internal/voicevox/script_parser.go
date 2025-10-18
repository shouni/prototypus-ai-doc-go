package voicevox

import (
	"bytes"
	"log/slog"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ----------------------------------------------------------------------
// 定数・変数
// ----------------------------------------------------------------------

const emotionTagsPattern = `(解説|疑問|驚き|理解|落ち着き|納得|断定|呼びかけ|まとめ|通常|喜び|怒り|ノーマル|あまあま|ツンツン|セクシー|ヒソヒソ|ささやき)`

var (
	reScriptParse  = regexp.MustCompile(`^(\[.+?\])\s*(\[.+?\])\s*(.*)`)
	reEmotionParse = regexp.MustCompile(`\[` + emotionTagsPattern + `\]`)

	// 最大テキスト長（文字数）。VOICEVOXが安全に処理できる最大文字数の目安として250文字に設定。
	maxSegmentCharLength = 250
)

// scriptSegment の定義は engine.go にあることを前提とします。

// ----------------------------------------------------------------------
// スクリプト解析ロジック
// ----------------------------------------------------------------------

// parseScript はスクリプトを話者・スタイルのタグが変わるか、最大文字数に達するまで結合します。
// ★ 修正: fallbackTagを受け取る引数を追加
func parseScript(script string, fallbackTag string) []scriptSegment {
	lines := bytes.Split([]byte(script), []byte("\n"))
	var segments []scriptSegment

	var currentTag string
	var currentText strings.Builder
	var textBuffer string // タグがない行の超過テキストを一時保持するバッファ

	// テキストを指定された最大文字数で安全に分割するヘルパー関数
	safeSplit := func(text string, currentLen int) (string, string) {
		// 現在のテキストの文字数 + スペース1文字 + 新しいテキスト の総文字数をチェック
		totalChars := utf8.RuneCountInString(currentText.String()) + 1 + utf8.RuneCountInString(text)

		if totalChars <= maxSegmentCharLength {
			// 制限内なら全て追加可能
			return text, ""
		}

		// 制限超過の場合、現在のセグメントの残り許容量を計算
		remainingCapacity := maxSegmentCharLength - (utf8.RuneCountInString(currentText.String()) + 1)

		if remainingCapacity <= 0 {
			// 既に currentText が最大値に近い場合、新しいテキスト全体を残りに回す
			return "", text
		}

		// 新しいテキストを指定された文字数で切り取る
		runes := []rune(text)

		// 切り取る文字数
		charsToTake := remainingCapacity
		if charsToTake > len(runes) {
			charsToTake = len(runes)
		}

		// 分割
		partToAdd := string(runes[:charsToTake])
		remainder := string(runes[charsToTake:])

		return partToAdd, remainder
	}

	// 結合されたセグメントを確定してリセットするヘルパー関数
	flushSegment := func(tag string, text string) {
		if text == "" || tag == "" {
			return
		}

		finalText := reEmotionParse.ReplaceAllString(text, "")
		finalText = strings.TrimSpace(finalText)

		if finalText != "" {
			segments = append(segments, scriptSegment{
				SpeakerTag: tag,
				Text:       finalText,
			})
		}
	}

	// 現在のセグメントを確定し、バッファをリセットする関数
	flushAndReset := func(currentTag string, currentText *strings.Builder) {
		if currentText.Len() > 0 && currentTag != "" {
			flushSegment(currentTag, currentText.String())
		}
		currentText.Reset()
	}

	// メインループ
	for _, lineBytes := range lines {
		line := string(bytes.TrimSpace(lineBytes))
		if line == "" {
			continue
		}

		textToProcess := line
		if textBuffer != "" {
			textToProcess = textBuffer + " " + line
			textBuffer = "" // バッファをクリア
		}

		matches := reScriptParse.FindStringSubmatch(textToProcess)

		if len(matches) > 3 {
			// ★ タグ行の処理
			speakerTag := matches[1]
			vvStyleTag := matches[2]
			textPart := matches[3]

			newCombinedTag := speakerTag + vvStyleTag
			fullTextToAppend := textPart

			// 1. タグ変更チェックと強制フラッシュ
			if currentTag != "" && newCombinedTag != currentTag {
				flushAndReset(currentTag, &currentText)
				currentTag = ""
			}

			// 2. 文字数制限チェックとセグメント化
			for fullTextToAppend != "" {
				if currentTag == "" {
					currentTag = newCombinedTag
				}

				partToAdd, remainder := safeSplit(fullTextToAppend, currentText.Len())

				if partToAdd != "" {
					if currentText.Len() > 0 {
						currentText.WriteString(" ")
					}
					currentText.WriteString(partToAdd)
				}

				if remainder != "" {
					slog.Warn("タグ付きテキストが最大文字数を超過したため、セグメントを強制的に確定し、残りのテキストを分割します。",
						"max_chars", maxSegmentCharLength,
						"tag", currentTag)

					flushAndReset(currentTag, &currentText)

					fullTextToAppend = remainder
					currentTag = newCombinedTag
				} else {
					fullTextToAppend = ""
				}
			}

		} else {
			// ★ タグがない行の処理

			if currentTag != "" {
				// 既存のセグメントに結合できる場合
				textToAppend := textToProcess

				for textToAppend != "" {
					partToAdd, remainder := safeSplit(textToAppend, currentText.Len())

					if partToAdd != "" {
						if currentText.Len() > 0 {
							currentText.WriteString(" ")
						}
						currentText.WriteString(partToAdd)
					}

					if remainder != "" {
						slog.Warn("タグのないテキスト行が最大文字数を超過したため、現在のセグメントを強制的に確定し、残りのテキストを分割します。",
							"tag", currentTag,
							"max_chars", maxSegmentCharLength)

						flushAndReset(currentTag, &currentText)
						textToAppend = remainder
					} else {
						textToAppend = ""
					}
				}

			} else {
				// currentTag が空の状態でタグなし行が来た
				textBuffer = textToProcess
				slog.Warn("タグのないテキスト行が検出されました。次のタグ付きセグメントに結合されます。", "text", line)
			}
		}
	}

	// ループ終了後、バッファに残っている最後のセグメントを確定
	flushAndReset(currentTag, &currentText)

	// ★ 修正ロジック: スクリプトの終端に残ったテキストバッファの処理
	if textBuffer != "" {

		if len(segments) > 0 {
			// 1. 既存のセグメントが存在する場合、最後のタグを流用
			lastTag := segments[len(segments)-1].SpeakerTag
			slog.Warn("スクリプトの最後にタグのないテキストが残りました。最後のタグを流用して最終セグメントとして合成します。",
				"lost_text", textBuffer,
				"used_tag", lastTag)

			flushSegment(lastTag, textBuffer)

		} else {
			// 2. 既存のセグメントが一つもない場合 (タグなしテキストのみのスクリプト)
			slog.Warn("スクリプトにタグ付きセグメントがありませんでした。デフォルトタグを使用してテキスト全体を合成します。",
				"text_content", textBuffer,
				"default_tag", fallbackTag)

			// ★ 修正: 設定可能なfallbackTagを使用
			if fallbackTag != "" {
				flushSegment(fallbackTag, textBuffer)
			} else {
				// デフォルトタグもない場合はエラー
				slog.Error("スクリプトに有効なタグがなく、フォールバックタグも設定されていません。テキストは合成されません。", "lost_text", textBuffer)
			}
		}
	}

	return segments
}
