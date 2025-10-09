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

// StyleIDMappings は、キャラクター名からVOICEVOXの話者スタイルIDへのマッピングです。
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

	segments := parseScript(scriptContent)
	if len(segments) == 0 {
		return fmt.Errorf("スクリプトから有効なセグメントを抽出できませんでした")
	}

	var audioDataList [][]byte

	for _, seg := range segments {
		if seg.Text == "" {
			continue
		}

		// 1. オーディオクエリ (audio_query) の作成
		queryURL := fmt.Sprintf("%s/audio_query", apiURL)

		styleID, ok := StyleIDMappings[seg.SpeakerTag]
		if !ok {
			return fmt.Errorf("話者タグ %s に対応するVOICEVOX Style IDが見つかりません", seg.SpeakerTag)
		}

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

		if resp.StatusCode != http.StatusOK {
			// APIがエラーを返す場合、レスポンスボディを読むとデバッグに役立つ
			errorBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("オーディオクエリAPIがエラーを返しました: Status %d, Body: %s", resp.StatusCode, string(errorBody))
		}

		queryBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("オーディオクエリボディの読み込み失敗: %w", err)
		}

		// 2. 音声合成 (synthesis) の実行
		synthURL := fmt.Sprintf("%s/synthesis", apiURL)

		synthParams := url.Values{}
		synthParams.Add("speaker", strconv.Itoa(styleID))

		synthResp, err := client.Post(synthURL+"?"+synthParams.Encode(), "application/json", bytes.NewReader(queryBody))
		if err != nil {
			return fmt.Errorf("音声合成API呼び出し失敗: %w", err)
		}

		if synthResp.StatusCode != http.StatusOK {
			errorBody, _ := io.ReadAll(synthResp.Body)
			synthResp.Body.Close()
			return fmt.Errorf("音声合成APIがエラーを返しました: Status %d, Body: %s", synthResp.StatusCode, string(errorBody))
		}

		wavData, err := io.ReadAll(synthResp.Body)
		synthResp.Body.Close()
		if err != nil {
			return fmt.Errorf("音声合成結果の読み込み失敗: %w", err)
		}

		audioDataList = append(audioDataList, wavData)
	}

	// 3. 音声ファイルのマージと保存 (WAV連結処理)
	combinedWavBytes, err := combineWavData(audioDataList)
	if err != nil {
		return fmt.Errorf("WAVデータの結合に失敗しました: %w", err)
	}

	return os.WriteFile(outputWavFile, combinedWavBytes, 0644)
}

// ----------------------------------------------------------------------
// ヘルパー構造体と関数
// ----------------------------------------------------------------------

type scriptSegment struct {
	SpeakerTag string
	Text       string
}

// parseScript はスクリプトから話者タグとテキストを抽出します。
func parseScript(script string) []scriptSegment {
	re := regexp.MustCompile(`(\[.+?\])\s*(.*)`)
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

			// 感情タグを除去し、不要な空白を削除
			text := reEmotion.ReplaceAllString(textWithEmotion, "")
			text = regexp.MustCompile(`\s{2,}`).ReplaceAllString(text, " ")
			text = regexp.MustCompile(`\s$`).ReplaceAllString(text, "")

			segments = append(segments, scriptSegment{
				SpeakerTag: speakerTag,
				Text:       text,
			})
		}
	}
	return segments
}

// combineWavData は複数の完全なWAVバイトデータを結合し、一つのWAVファイルを返します。
// VOICEVOXが出力する標準的なWAV構造に依存した簡易連結処理です。
func combineWavData(wavFiles [][]byte) ([]byte, error) {
	var rawData []byte
	totalDataSize := uint32(0)

	// WAVヘッダーのサイズ (RIFF ID, File Size, WAVE ID, FMT ID, FMT Size, FMT Contents)
	const HeaderSize = 44

	// 最初のファイルからフォーマット情報（最初の36バイト）を取得
	if len(wavFiles[0]) < HeaderSize-8 { // 36バイト
		return nil, fmt.Errorf("最初のWAVファイルのヘッダーが短すぎます")
	}
	formatHeader := wavFiles[0][0 : HeaderSize-8]

	for _, wavBytes := range wavFiles {
		if len(wavBytes) < HeaderSize {
			return nil, fmt.Errorf("WAVファイルがデータチャンクを含んでいません")
		}

		// Dataチャンクのサイズを取得 (バイト40-43)
		dataSize := uint32(wavBytes[40]) | uint32(wavBytes[41])<<8 | uint32(wavBytes[42])<<16 | uint32(wavBytes[43])<<24

		// dataチャンクの内容を取得 (44バイト目からデータサイズ分)
		dataChunk := wavBytes[44 : 44+dataSize]

		rawData = append(rawData, dataChunk...)
		totalDataSize += dataSize
	}

	// 新しいWAVファイルを構築
	combinedWav := make([]byte, HeaderSize+totalDataSize)
	copy(combinedWav, formatHeader) // 最初の36バイト (RIFF, WAVE, FMTチャンク) をコピー

	// "data" チャンクの識別子とサイズを書き込む (バイト36から)
	copy(combinedWav[36:], []byte("data"))
	combinedWav[40] = byte(totalDataSize)
	combinedWav[41] = byte(totalDataSize >> 8)
	combinedWav[42] = byte(totalDataSize >> 16)
	combinedWav[43] = byte(totalDataSize >> 24)

	// ファイル全体のサイズを更新 (バイト4から)
	// FileSize = 44 (ヘッダー) + totalDataSize - 8 (RIFFチャンクのIDとサイズ分)
	fileSize := HeaderSize + totalDataSize - 8
	combinedWav[4] = byte(fileSize)
	combinedWav[5] = byte(fileSize >> 8)
	combinedWav[6] = byte(fileSize >> 16)
	combinedWav[7] = byte(fileSize >> 24)

	// 結合したデータをコピー (バイト44から)
	copy(combinedWav[44:], rawData)

	return combinedWav, nil
}
