package voicevox

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"
)

// VOICEVOX_API_URL 環境変数でVOICEVOXエンジンのURLを指定します
// 例: http://localhost:50021

// StyleIDMappings は、キャラクター名からVOICEVOXの話者スタイルIDへのマッピングです。
// これはツールのプロンプトが返すタグに対応している必要があります。
// 以下のIDはVOICEVOX公式の「ずんだもん (ノーマル)」と「四国めたん (ノーマル)」のデフォルトIDを仮定しています。
var StyleIDMappings = map[string]int{
	"[ずんだもん]": 3, // ずんだもん (ノーマル)
	"[めたん]":   2, // 四国めたん (ノーマル)
}

// PostToEngine はスクリプト全体をVOICEVOXエンジンに投稿し、音声ファイルを生成します。
func PostToEngine(scriptContent string, outputWavFile string) error {
	apiURL := os.Getenv("VOICEVOX_API_URL")
	if apiURL == "" {
		return fmt.Errorf("VOICEVOX_API_URL 環境変数が設定されていません")
	}

	// タイムアウトを設定したHTTPクライアント
	client := &http.Client{
		Timeout: 30 * time.Second, // 合成処理は時間がかかるため、長めに設定
	}

	// 1. スクリプトを行ごとに処理し、合成
	segments := parseScript(scriptContent)
	var audioDataList [][]byte

	for _, seg := range segments {
		if seg.Text == "" {
			continue
		}

		// 2. オーディオクエリ (Query) の作成
		queryURL := fmt.Sprintf("%s/audio_query", apiURL)

		// StyleIDの取得とチェック
		styleID, ok := StyleIDMappings[seg.SpeakerTag]
		if !ok {
			return fmt.Errorf("話者タグ %s に対応するVOICEVOX Style IDが見つかりません", seg.SpeakerTag)
		}

		// クエリパラメータの設定 (textとspeakerをURLエンコード)
		params := url.Values{}
		params.Add("text", seg.Text)
		params.Add("speaker", strconv.Itoa(styleID))

		req, err := http.NewRequest("POST", queryURL+"?"+params.Encode(), nil)
		if err != nil {
			return fmt.Errorf("オーディオクエリPOSTリクエスト作成失敗: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("オーディオクエリAPI呼び出し失敗: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("オーディオクエリAPIがエラーを返しました: Status %d", resp.StatusCode)
		}

		queryBody, _ := io.ReadAll(resp.Body) // クエリパラメータ(JSON)を取得

		// 3. 合成 (Synthesis) の実行
		synthURL := fmt.Sprintf("%s/synthesis", apiURL)

		// synthesisリクエストのパラメータ設定
		synthParams := url.Values{}
		synthParams.Add("speaker", strconv.Itoa(styleID))

		// synthesis APIにクエリJSONとspeaker IDを送信
		synthResp, err := client.Post(synthURL+"?"+synthParams.Encode(), "application/json", bytes.NewReader(queryBody))
		if err != nil {
			return fmt.Errorf("音声合成API呼び出し失敗: %w", err)
		}
		defer synthResp.Body.Close()

		if synthResp.StatusCode != http.StatusOK {
			return fmt.Errorf("音声合成APIがエラーを返しました: Status %d", synthResp.StatusCode)
		}

		wavData, err := io.ReadAll(synthResp.Body)
		if err != nil {
			return fmt.Errorf("音声合成結果の読み込み失敗: %w", err)
		}
		audioDataList = append(audioDataList, wavData)
	}

	// 4. 音声ファイルのマージと保存 (実装簡略化のため、ここでは最初のセグメントのみを保存)
	// ★ 注意: 複数のWAVファイルを連結するには、WAVヘッダー構造を理解した処理が必要です。
	// この例では、生成された最後の音声データのみをファイルに保存する形で簡略化します。
	if len(audioDataList) > 0 {
		finalWavData := audioDataList[len(audioDataList)-1]
		return os.WriteFile(outputWavFile, finalWavData, 0644)
	}

	return nil
}

// --- 内部ヘルパー関数 ---

type scriptSegment struct {
	SpeakerTag string
	Text       string
}

// parseScript はスクリプトから話者タグとテキストを抽出します。
// 例: "[ずんだもん] こんにちはなのだ！[喜び]" -> {SpeakerTag: "[ずんだもん]", Text: "こんにちはなのだ！"}
func parseScript(script string) []scriptSegment {
	// 話者タグの正規表現: [話者名]
	re := regexp.MustCompile(`(\[.+?\])\s*(.*)`)

	// 感情タグの正規表現: [感情] を除去
	reEmotion := regexp.MustCompile(`\[.+?\]`)

	lines := bytes.Split([]byte(script), []byte("\n"))
	var segments []scriptSegment

	for _, lineBytes := range lines {
		line := string(bytes.TrimSpace(lineBytes))
		if line == "" {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) > 2 {
			speakerTag := matches[1]
			textWithEmotion := matches[2]

			// 感情タグを除去
			text := reEmotion.ReplaceAllString(textWithEmotion, "")
			text = regexp.MustCompile(`\s{2,}`).ReplaceAllString(text, " ") // 過剰な空白を一つにまとめる
			text = regexp.MustCompile(`\s$`).ReplaceAllString(text, "")     // 文末の空白を除去

			segments = append(segments, scriptSegment{
				SpeakerTag: speakerTag,
				Text:       text,
			})
		}
	}
	return segments
}
