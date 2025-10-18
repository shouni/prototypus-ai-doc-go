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

	// 最大テキスト長（文字数）
	maxSegmentCharLength = 250

	// ★ 修正: タグがない場合にフォールバックするためのハードコードされたデフォルトタグ
	// 実際の運用では、このタグは設定ファイルや外部データから取得すべきです。
	fallbackDefaultTag = "[四国めたん][ノーマル]"
)

// scriptSegment の定義は engine.go にあることを前提とします。

// ----------------------------------------------------------------------
// スクリプト解析ロジック
// ----------------------------------------------------------------------

// parseScript はスクリプトを話者・スタイルのタグが変わるか、最大文字数に達するまで結合します。
func parseScript(script string) []scriptSegment {
	lines := bytes.Split([]byte(script), []byte("\n"))
	var segments []scriptSegment

	var currentTag string
	var currentText strings.Builder
	var textBuffer string // タグがない行の超過テキストを一時保持するバッファ

	// テキストを指定された最大文字数で安全に分割するヘルパー関数
	safeSplit := func(text string, currentLen int) (string, string) {
		// ... (safeSplit関数の実装は省略。文字数ベースで安全に分割するロジック)
		totalChars := utf8.RuneCountInString(currentText.String()) + 1 + utf8.RuneCountInString(text)
		if totalChars <= maxSegmentCharLength {
			return text, ""
		}
		remainingCapacity := maxSegmentCharLength - (utf8.RuneCountInString(currentText.String()) + 1)
		if remainingCapacity <= 0 {
			return "", text
		}
		runes := []rune(text)
		charsToTake := remainingCapacity
		if charsToTake > len(runes) {
			charsToTake = len(runes)
		}
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

		// textBufferに何か残っている場合、それを現在の行の前に結合する
		textToProcess := line
		if textBuffer != "" {
			// 前の行と今回の行を結合。タグなしテキストも結合ロジックに乗せる
			textToProcess = textBuffer + " " + line
			textBuffer = "" // バッファをクリア
		}

		matches := reScriptParse.FindStringSubmatch(textToProcess)

		if len(matches) > 3 {
			// ★ タグ行の処理 (ロジックは前回修正版と同じで安定)
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
			// ★ タグがない行の処理 (textToProcessには line or textBuffer + line が入っている)

			if currentTag != "" {
				// 既存のセグメントに結合できる場合
				textToAppend := textToProcess // textToProcess は今回の行か、前の超過テキスト+今回の行

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
				// currentTag が空 (スクリプトの先頭、またはflush直後) の状態でタグなし行が来た
				// ★ 修正: textBufferにテキストを蓄積する (textToProcessは既に textBuffer + line の状態かもしれない)
				textBuffer = textToProcess
				slog.Warn("タグのないテキスト行が検出されました。次のタグ付きセグメントに結合されます。", "text", line)
			}
		}
	}

	// ループ終了後、バッファに残っている最後のセグメントを確定
	flushAndReset(currentTag, &currentText)

	// ★ 修正ロジック: スクリプトの終端に残ったテキストバッファの処理
	if textBuffer != "" {
		// タグなしテキストが残った場合

		// 1. 既存のセグメントが存在する場合、最後のタグを流用する
		if len(segments) > 0 {
			lastTag := segments[len(segments)-1].SpeakerTag
			slog.Warn("スクリプトの最後にタグのないテキストが残りました。最後のタグを流用して最終セグメントとして合成します。",
				"lost_text", textBuffer,
				"used_tag", lastTag)

			flushSegment(lastTag, textBuffer)

		} else {
			// 2. 既存のセグメントが一つもない場合 (タグなしテキストのみのスクリプト)
			slog.Warn("スクリプトにタグ付きセグメントがありませんでした。デフォルトタグを使用してテキスト全体を合成します。",
				"text_content", textBuffer,
				"default_tag", fallbackDefaultTag)

			// ★ クリティカルバグ修正: デフォルトタグを使用してテキストを合成
			if fallbackDefaultTag != "" {
				flushSegment(fallbackDefaultTag, textBuffer)
			} else {
				// デフォルトタグもない場合はエラー
				slog.Error("スクリプトに有効なタグがなく、フォールバックタグも設定されていません。テキストは合成されません。", "lost_text", textBuffer)
			}
		}
	}

	// PostToEngineに渡すsegmentsが空の場合、PostToEngine側でエラーを返す（最初のチェックで対応済み）
	return segments
}
