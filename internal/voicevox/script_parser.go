package voicevox

import (
	"bytes"
	"regexp"
	"strings"
)

// ----------------------------------------------------------------------
// 演出用感情タグの定義 (このタグは音声合成時にはテキストから除去される)
// ----------------------------------------------------------------------

// 許可された感情タグとスタイルタグのリスト (正規表現で除去するために使用)
// VOICEVOXトーンタグ（[ノーマル]など）も、3番目の位置に誤って挿入された場合に除去するため含める。
const emotionTagsPattern = `(解説|疑問|驚き|理解|落ち着き|断定|呼びかけ|納得|通常|喜び|怒り|ノーマル|あまあま|ツンツン|セクシー|ヒソヒソ|ささやき)`

// ----------------------------------------------------------------------
// スクリプト解析ロジック
// ----------------------------------------------------------------------

// scriptSegment は engine.go で定義されたものを利用します。
// [話者タグ][スタイルタグ] [演出タグ] テキスト の形式を想定しています。
// Note: この関数は形式の解析のみを行い、タグの有効性チェックは engine.go に委譲します。
func parseScript(script string) []scriptSegment {
	// 最初の2つのタグを抽出する正規表現（例: [ずんだもん][ノーマル]）。
	re := regexp.MustCompile(`^(\[.+?\])\s*(\[.+?\])\s*(.*)`)

	// 感情タグと誤挿入されたスタイルタグを除去するための正規表現
	// 例: [解説]、[納得]、[通常]、[ツンツン] など
	reEmotion := regexp.MustCompile(`\[` + emotionTagsPattern + `\]`)

	lines := bytes.Split([]byte(script), []byte("\n"))
	var segments []scriptSegment

	for _, lineBytes := range lines {
		line := string(bytes.TrimSpace(lineBytes))
		if line == "" {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) > 3 {
			speakerTag := matches[1]
			vvStyleTag := matches[2]
			textWithEmotion := matches[3]

			combinedTag := speakerTag + vvStyleTag

			// 3番目の位置に存在するタグ（演出タグまたは誤挿入されたタグ）を除去
			text := reEmotion.ReplaceAllString(textWithEmotion, "")
			text = strings.TrimSpace(text)

			if text != "" {
				segments = append(segments, scriptSegment{
					SpeakerTag: combinedTag,
					Text:       text,
				})
			}
		}
		// マッチしない行は無視（不正なフォーマットとみなす）
	}
	return segments
}
