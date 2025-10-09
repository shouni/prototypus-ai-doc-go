package voicevox

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"
)

// 話者タグの定数定義
const (
	SpeakerTagZundamon = "[ずんだもん]"
	SpeakerTagMetan    = "[めたん]"
)

// StyleIDMappings は、キャラクター名からVOICEVOXの話者スタイルIDへのマッピングです。
var StyleIDMappings = map[string]int{
	SpeakerTagZundamon: 3, // ずんだもん (ノーマル)
	SpeakerTagMetan:    2, // 四国めたん (ノーマル)
}

// WAVヘッダーのマジックナンバーの定数化
const (
	// WAV/RIFF チャンク
	RiffChunkIDSize    = 4                                                 // "RIFF"
	RiffChunkSizeField = 4                                                 // File Size (全体のサイズ - 8)
	WaveIDSize         = 4                                                 // "WAVE"
	WavRiffHeaderSize  = RiffChunkIDSize + RiffChunkSizeField + WaveIDSize // 12 bytes

	// FMT チャンク
	FmtChunkIDSize    = 4                                                     // "fmt "
	FmtChunkSizeField = 4                                                     // fmt Chunk Size (16 for PCM)
	FmtChunkDataSize  = 16                                                    // Format Tag, Channels, SampleRate, etc.
	WavFmtChunkSize   = FmtChunkIDSize + FmtChunkSizeField + FmtChunkDataSize // 24 bytes

	// DATA チャンク
	DataChunkIDSize    = 4                                    // "data"
	DataChunkSizeField = 4                                    // Data Size (データ本体のサイズ)
	WavDataHeaderSize  = DataChunkIDSize + DataChunkSizeField // 8 bytes

	// 合計ヘッダーサイズ: 12 + 24 + 8 = 44 bytes
	WavTotalHeaderSize = WavRiffHeaderSize + WavFmtChunkSize + WavDataHeaderSize
)

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

		// --- 修正箇所 ---
		resp, err := client.Do(req) // ★ resp の定義
		if err != nil {
			return fmt.Errorf("オーディオクエリAPI呼び出し失敗: %w", err)
		}
		defer resp.Body.Close() // ★ 定義直後に defer を配置

		if resp.StatusCode != http.StatusOK {
			// APIがエラーを返す場合、レスポンスボディを読む
			errorBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("オーディオクエリAPIがエラーを返しました: Status %d, Body: %s", resp.StatusCode, string(errorBody))
		}

		queryBody, err := io.ReadAll(resp.Body)
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
		defer synthResp.Body.Close() // ★ 定義直後に defer を配置

		if synthResp.StatusCode != http.StatusOK {
			errorBody, _ := io.ReadAll(synthResp.Body)
			// 手動の synthResp.Body.Close() は defer のため削除
			return fmt.Errorf("音声合成APIがエラーを返しました: Status %d, Body: %s", synthResp.StatusCode, string(errorBody))
		}

		wavData, err := io.ReadAll(synthResp.Body)
		// 手動の synthResp.Body.Close() は defer のため削除
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
// VOICEVOXが出力する標準的なWAV構造（例: PCM、44.1kHz、16bit、モノラル）に依存した簡易連結処理です。
func combineWavData(wavFiles [][]byte) ([]byte, error) {
	var rawData []byte
	totalDataSize := uint32(0)

	// 最初のファイルからフォーマット情報（最初の36バイト）を取得
	fmtChunkEndIndex := WavRiffHeaderSize + WavFmtChunkSize
	if len(wavFiles[0]) < fmtChunkEndIndex {
		return nil, fmt.Errorf("最初のWAVファイルのヘッダー（RIFF + FMT）が短すぎます")
	}
	formatHeader := wavFiles[0][0:fmtChunkEndIndex] // 最初の36バイト (RIFFヘッダー + FMTチャンク)

	for _, wavBytes := range wavFiles {
		if len(wavBytes) < WavTotalHeaderSize {
			return nil, fmt.Errorf("WAVファイルが完全なヘッダーを含んでいません")
		}

		// Dataチャンクのサイズを取得 (バイト40-43)
		dataSizeStartIndex := WavTotalHeaderSize - DataChunkSizeField // 40バイト目
		dataSize := binary.LittleEndian.Uint32(wavBytes[dataSizeStartIndex:WavTotalHeaderSize])

		// dataチャンクの内容を取得 (44バイト目からデータサイズ分)
		dataChunk := wavBytes[WavTotalHeaderSize : WavTotalHeaderSize+dataSize]

		rawData = append(rawData, dataChunk...)
		totalDataSize += dataSize
	}

	// 新しいWAVファイルを構築
	combinedWav := make([]byte, WavTotalHeaderSize+totalDataSize)
	copy(combinedWav, formatHeader) // 最初の36バイト (RIFF, WAVE, FMTチャンク) をコピー

	// "data" チャンクの識別子とサイズを書き込む (バイト36から)
	dataIDStartIndex := WavRiffHeaderSize + WavFmtChunkSize // 36バイト目
	copy(combinedWav[dataIDStartIndex:], []byte("data"))

	// Data Chunk Size (バイト40-43)を更新
	dataSizeStartIndex := WavTotalHeaderSize - DataChunkSizeField // 40バイト目
	binary.LittleEndian.PutUint32(combinedWav[dataSizeStartIndex:WavTotalHeaderSize], totalDataSize)

	// ファイル全体のサイズを更新 (バイト4から)
	// FileSize = WavTotalHeaderSize + totalDataSize - 8 (RIFF IDとFileSizeフィールドの8バイトを引く)
	fileSize := WavTotalHeaderSize + totalDataSize - WavRiffHeaderSize + RiffChunkSizeField
	fileSizeStartIndex := RiffChunkIDSize // 4バイト目
	binary.LittleEndian.PutUint32(combinedWav[fileSizeStartIndex:fileSizeStartIndex+RiffChunkSizeField], fileSize)

	// 結合したデータをコピー (バイト44から)
	copy(combinedWav[WavTotalHeaderSize:], rawData)

	return combinedWav, nil
}
