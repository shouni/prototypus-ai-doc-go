package voicevox

import (
	"bytes"
	"log/slog"
	"regexp"
	"strings"
	"unicode/utf8" // ★ 追加: バイト数ではなく文字数での正確な分割をサポート
)

// ----------------------------------------------------------------------
// 定数・変数
// ----------------------------------------------------------------------

const emotionTagsPattern = `(解説|疑問|驚き|理解|落ち着き|納得|断定|呼びかけ|まとめ|通常|喜び|怒り|ノーマル|あまあま|ツンツン|セクシー|ヒソヒソ|ささやき)`

var (
	reScriptParse  = regexp.MustCompile(`^(\[.+?\])\s*(\[.+?\])\s*(.*)`)
	reEmotionParse = regexp.MustCompile(`\[` + emotionTagsPattern + `\]`)
	// ★ 修正: バイト数ではなく、VOICEVOXが安全に処理できる「文字数」の目安 (日本語で約250〜300文字) を設定。
	// UTF-8ベースのGoではバイト数で管理するが、コメントで文字数の意図を明確化。
	// VOICEVOXの公式制限は通常500文字だが、安全を見て250文字に設定。
	maxSegmentCharLength = 250
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

	// 結合されたセグメントを確定してリセットするヘルパー関数
	flushSegment := func(tag string, text string) {
		if text == "" || tag == "" {
			return
		}

		// 感情タグを除去し、スペースをトリム
		finalText := reEmotionParse.ReplaceAllString(text, "")
		finalText = strings.TrimSpace(finalText)

		if finalText != "" {
			segments = append(segments, scriptSegment{
				SpeakerTag: tag,
				Text:       finalText,
			})
		}
	}

	// テキストを指定された最大文字数で安全に分割するヘルパー関数
	// 戻り値: [0]セグメントに追加する部分, [1]残りの超過部分
	safeSplit := func(text string, currentLen int) (string, string) {
		// 現在のテキストの文字数 + スペース1文字 + 新しいテキスト の総文字数をチェック
		totalChars := utf8.RuneCountInString(currentText.String()) + 1 + utf8.RuneCountInString(text)

		if totalChars <= maxSegmentCharLength {
			// 制限内なら全て追加可能
			return text, ""
		}

		// 制限超過の場合、現在のセグメントの残り許容量を計算
		// maxSegmentCharLength - (currentTextの文字数 + スペース1文字)
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

	// 現在のセグメントを確定し、バッファをリセットする関数（カスタム版）
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
			textToProcess = textBuffer + " " + line
			textBuffer = "" // バッファをクリア
		}

		matches := reScriptParse.FindStringSubmatch(textToProcess)

		if len(matches) > 3 {
			// ★ タグ行の処理
			speakerTag := matches[1]
			vvStyleTag := matches[2]
			textPart := matches[3] // タグ行のテキスト部分

			newCombinedTag := speakerTag + vvStyleTag

			// 処理対象テキスト: タグ行のテキスト部分
			fullTextToAppend := textPart

			// 1. タグ変更チェックと強制フラッシュ
			if currentTag != "" && newCombinedTag != currentTag {
				// タグが変わった場合、古いセグメントを確定
				flushAndReset(currentTag, &currentText)
				currentTag = ""
			}

			// 2. 文字数制限チェックとセグメント化
			for fullTextToAppend != "" {
				// currentTagが空の場合、新しいセグメントとして開始
				if currentTag == "" {
					currentTag = newCombinedTag
				}

				// 現在のテキスト許容量内で分割を試みる
				partToAdd, remainder := safeSplit(fullTextToAppend, currentText.Len())

				if partToAdd != "" {
					// セグメントに追加
					if currentText.Len() > 0 {
						currentText.WriteString(" ")
					}
					currentText.WriteString(partToAdd)
				}

				if remainder != "" {
					// 超過した場合、現在のセグメントを確定し、残りを次のセグメントとして処理
					slog.Warn("セグメントの最大文字数を超過しました。現在のセグメントを強制的に確定し、残りのテキストを分割します。",
						"max_chars", maxSegmentCharLength,
						"tag", currentTag)

					flushAndReset(currentTag, &currentText)

					// 残りのテキストを次のループで処理するためにセット
					fullTextToAppend = remainder
					currentTag = newCombinedTag // 新しいセグメントも同じタグで開始
				} else {
					// 制限内に収まったらループを抜ける
					fullTextToAppend = ""
				}
			}

		} else if currentTag != "" {
			// ★ タグがない行（前のタグを引き継いで結合）

			textToAppend := line // タグなしの行全体を処理対象とする

			for textToAppend != "" {
				// 文字数制限内で分割を試みる
				partToAdd, remainder := safeSplit(textToAppend, currentText.Len())

				if partToAdd != "" {
					// セグメントに追加
					if currentText.Len() > 0 {
						currentText.WriteString(" ")
					}
					currentText.WriteString(partToAdd)
				}

				if remainder != "" {
					// 超過した場合、現在のセグメントを確定し、残りを次のセグメントとして処理
					slog.Warn("タグのないテキスト行が最大文字数を超過したため、現在のセグメントを強制的に確定し、残りのテキストを分割します。",
						"tag", currentTag,
						"max_chars", maxSegmentCharLength)

					flushAndReset(currentTag, &currentText)

					// 残りのテキストを次のループで処理するためにセット
					textToAppend = remainder
					// currentTagは継続 (タグなし行は前のタグを引き継ぐ)
				} else {
					// 制限内に収まったらループを抜ける
					textToAppend = ""
				}
			}
		} else {
			// タグがない行が来て、かつ currentTag も空（スクリプトの先頭、またはflush直後のタグなし行）
			// このテキストは textBuffer に残しておく
			textBuffer = textToProcess
			slog.Warn("タグのないテキスト行が検出されました。次のタグ付きセグメントに結合されます。", "text", line)
		}
	}

	// ループ終了後、バッファに残っている最後のセグメントを確定
	flushAndReset(currentTag, &currentText)

	// 最後に textBuffer に何かが残っている場合、テキストロストの警告
	if textBuffer != "" {
		if len(segments) > 0 {
			// タグ付きセグメントが存在する場合、最後のセグメントのタグを流用してテキストを合成
			lastTag := segments[len(segments)-1].SpeakerTag
			slog.Warn("スクリプトの最後にタグのないテキストが残りました。最後のタグを流用して最終セグメントとして合成します。",
				"lost_text", textBuffer,
				"used_tag", lastTag)

			flushSegment(lastTag, textBuffer)

		} else {
			// スクリプト全体にタグ付きセグメントが一つもない場合
			slog.Error("スクリプトに有効なタグ付きセグメントがない状態でタグのないテキストが残りました。テキストは合成されません。", "lost_text", textBuffer)
		}
	}

	return segments
}
