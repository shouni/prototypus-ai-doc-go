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
	"strings"
	"sync"
	"time"
)

// ----------------------------------------------------------------------
// 話者タグとVOICEVOXスタイルタグの定数定義
// ----------------------------------------------------------------------

// 話者タグの定数定義
const (
	SpeakerTagZundamon = "[ずんだもん]"
	SpeakerTagMetan    = "[めたん]"
)

// VOICEVOXのスタイルIDに対応する感情タグ
const (
	VvTagNormal  = "[通常]" // デフォルトスタイル
	VvTagHappy   = "[喜び]"
	VvTagAngry   = "[怒り]"
	VvTagWhisper = "[ささやき]"
)

// StyleIDMappings は、"話者タグ + スタイルタグ" の組み合わせからVOICEVOXのスタイルIDへのマッピングです。
var StyleIDMappings = map[string]int{
	// ずんだもん
	SpeakerTagZundamon + VvTagNormal:  3,
	SpeakerTagZundamon + VvTagHappy:   1,
	SpeakerTagZundamon + VvTagAngry:   4,
	SpeakerTagZundamon + VvTagWhisper: 18,
	// 四国めたん
	SpeakerTagMetan + VvTagNormal: 2,
	SpeakerTagMetan + VvTagHappy:  15,
	SpeakerTagMetan + VvTagAngry:  17,
}

// ----------------------------------------------------------------------
// 定数 (変更なし)
// ----------------------------------------------------------------------

const (
	RiffChunkIDSize    = 4
	RiffChunkSizeField = 4
	WaveIDSize         = 4
	WavRiffHeaderSize  = RiffChunkIDSize + RiffChunkSizeField + WaveIDSize // 12 bytes
	FmtChunkIDSize     = 4
	FmtChunkSizeField  = 4
	FmtChunkDataSize   = 16
	WavFmtChunkSize    = FmtChunkIDSize + FmtChunkSizeField + FmtChunkDataSize // 24 bytes
	DataChunkIDSize    = 4
	DataChunkSizeField = 4
	WavDataHeaderSize  = DataChunkIDSize + DataChunkSizeField // 8 bytes
	WavTotalHeaderSize = WavRiffHeaderSize + WavFmtChunkSize + WavDataHeaderSize
)

// ----------------------------------------------------------------------
// 並列処理用の構造体
// ----------------------------------------------------------------------

type scriptSegment struct {
	// SpeakerTag は [話者タグ][スタイルタグ] の結合タグ
	SpeakerTag string
	Text       string
}

type resultSegment struct {
	index   int
	wavData []byte
}

// ----------------------------------------------------------------------
// メイン処理 (並列化済み)
// ----------------------------------------------------------------------

// PostToEngine はスクリプト全体をVOICEVOXエンジンに投稿し、音声ファイルを生成します。
func PostToEngine(scriptContent string, outputWavFile string) error {
	apiURL := os.Getenv("VOICEVOX_API_URL")
	if apiURL == "" {
		return fmt.Errorf("VOICEVOX_API_URL 環境変数が設定されていません")
	}

	client := &http.Client{
		Timeout: 180 * time.Second,
	}

	segments := parseScript(scriptContent)
	if len(segments) == 0 {
		return fmt.Errorf("スクリプトから有効なセグメントを抽出できませんでした。AIの出力形式が [話者タグ][スタイルタグ] テキスト の形式に沿っているか確認してください")
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(segments))
	resultsChan := make(chan resultSegment, len(segments))

	const maxConcurrency = 10
	semaphore := make(chan struct{}, maxConcurrency)

	for i, seg := range segments {
		if seg.Text == "" {
			continue
		}

		semaphore <- struct{}{}
		wg.Add(1)

		go func(i int, seg scriptSegment) {
			defer wg.Done()
			defer func() { <-semaphore }()

			// スタイルIDの検索とフォールバック処理

			// 1. [話者タグ][スタイルタグ] の複合キーから ID を検索
			styleID, ok := StyleIDMappings[seg.SpeakerTag]

			// 2. IDが見つからなかった場合のフォールバック処理
			if !ok {
				// seg.SpeakerTag は "[話者タグ][スタイルタグ]" なので、最初のタグ ([話者タグ]) を抽出する
				// 正規表現で再抽出
				reSpeaker := regexp.MustCompile(`^(\[.+?\])`)
				speakerMatch := reSpeaker.FindStringSubmatch(seg.SpeakerTag)

				if len(speakerMatch) < 2 {
					errChan <- fmt.Errorf("話者タグ %s の解析に失敗しました (セグメント %d)", seg.SpeakerTag, i)
					return
				}

				baseSpeakerTag := speakerMatch[1] // 例: "[めたん]"

				// デフォルトのスタイル ([通常]) にフォールバックしたキーを作成
				fallbackKey := baseSpeakerTag + VvTagNormal // 例: "[めたん][通常]"

				defaultStyleID, defaultOk := StyleIDMappings[fallbackKey]

				if defaultOk {
					styleID = defaultStyleID
					// ログ出力などを行うと親切
					fmt.Printf("警告: スタイルタグ %s が見つかりません。デフォルトの %s にフォールバックします (セグメント %d)\n", seg.SpeakerTag, fallbackKey, i)
				} else {
					// デフォルトスタイルも見つからなければエラー
					errChan <- fmt.Errorf("話者・スタイルタグ %s (およびデフォルトの %s) に対応するVOICEVOX Style IDが見つかりません (セグメント %d)", seg.SpeakerTag, fallbackKey, i)
					return
				}
			}

			// IDが見つかった or フォールバックで取得できたので処理続行
			// 1. オーディオクエリ (audio_query) の作成
			queryBody, err := runAudioQuery(client, apiURL, seg.Text, styleID)
			if err != nil {
				errChan <- fmt.Errorf("セグメント %d のオーディオクエリ失敗: %w", i, err)
				return
			}

			// 2. 音声合成 (synthesis) の実行
			wavData, err := runSynthesis(client, apiURL, queryBody, styleID)
			if err != nil {
				errChan <- fmt.Errorf("セグメント %d の音声合成失敗: %w", i, err)
				return
			}

			resultsChan <- resultSegment{index: i, wavData: wavData}

		}(i, seg)
	}

	wg.Wait()
	close(resultsChan)
	close(errChan)

	if err := <-errChan; err != nil {
		return err
	}

	orderedAudioDataList := make([][]byte, len(segments))
	for res := range resultsChan {
		if res.index >= 0 && res.index < len(segments) {
			orderedAudioDataList[res.index] = res.wavData
		}
	}

	finalAudioDataList := make([][]byte, 0, len(segments))
	for _, data := range orderedAudioDataList {
		if data != nil {
			finalAudioDataList = append(finalAudioDataList, data)
		}
	}

	if len(finalAudioDataList) == 0 {
		return fmt.Errorf("すべてのセグメントの合成に失敗したか、有効なセグメントがありませんでした")
	}

	combinedWavBytes, err := combineWavData(finalAudioDataList)
	if err != nil {
		return fmt.Errorf("WAVデータの結合に失敗しました: %w", err)
	}

	return os.WriteFile(outputWavFile, combinedWavBytes, 0644)
}

// ----------------------------------------------------------------------
// ヘルパー関数 (API呼び出し、WAV結合は変更なし)
// ----------------------------------------------------------------------

// runAudioQuery は /audio_query APIを呼び出し、クエリボディを返します。
func runAudioQuery(client *http.Client, apiURL string, text string, styleID int) ([]byte, error) {
	queryURL := fmt.Sprintf("%s/audio_query", apiURL)
	params := url.Values{}
	params.Add("text", text)
	params.Add("speaker", strconv.Itoa(styleID))

	req, err := http.NewRequest("POST", queryURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("オーディオクエリPOSTリクエスト作成失敗: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("オーディオクエリAPI呼び出し失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("オーディオクエリAPIがエラーを返しました: Status %d, Body: %s", resp.StatusCode, string(errorBody))
	}

	queryBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("オーディオクエリボディの読み込み失敗: %w", err)
	}

	return queryBody, nil
}

// runSynthesis は /synthesis APIを呼び出し、WAVデータを返します。
func runSynthesis(client *http.Client, apiURL string, queryBody []byte, styleID int) ([]byte, error) {
	synthURL := fmt.Sprintf("%s/synthesis", apiURL)
	synthParams := url.Values{}
	synthParams.Add("speaker", strconv.Itoa(styleID))

	synthResp, err := client.Post(synthURL+"?"+synthParams.Encode(), "application/json", bytes.NewReader(queryBody))
	if err != nil {
		return nil, fmt.Errorf("音声合成API呼び出し失敗: %w", err)
	}
	defer synthResp.Body.Close()

	if synthResp.StatusCode != http.StatusOK {
		errorBody, _ := io.ReadAll(synthResp.Body)
		return nil, fmt.Errorf("音声合成APIがエラーを返しました: Status %d, Body: %s", synthResp.StatusCode, string(errorBody))
	}

	wavData, err := io.ReadAll(synthResp.Body)
	if err != nil {
		return nil, fmt.Errorf("音声合成結果の読み込み失敗: %w", err)
	}

	return wavData, nil
}

// parseScript はスクリプトから話者タグ、VOICEVOXスタイルタグ、およびテキストを抽出します。
func parseScript(script string) []scriptSegment {
	// AIの出力形式を厳密にパースする正規表現: [話者タグ][VOICEVOXスタイルタグ] (演出タグとテキスト)
	// $1: [話者タグ]
	// $2: [VOICEVOXスタイルタグ] (例: [通常], [喜び], [理解]などのAIが生成したカスタムタグも含む)
	// $3: スタイルタグ以降の全て (演出タグとテキスト)
	re := regexp.MustCompile(`^(\[.+?\])\s*(\[.+?\])\s*(.*)`)

	// 演出タグ除去用の正規表現 (VOICEVOXスタイルタグではないタグを全て削除)
	reEmotion := regexp.MustCompile(`\[.+?\]`)

	lines := bytes.Split([]byte(script), []byte("\n"))
	var segments []scriptSegment

	for _, lineBytes := range lines {
		line := string(bytes.TrimSpace(lineBytes))
		if line == "" {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) > 3 {
			speakerTag := matches[1] // [ずんだもん]
			vvStyleTag := matches[2] // [通常] や [理解] などのカスタムタグ

			// StyleIDMappingsで使うキー: [ずんだもん][通常] のように結合する
			combinedTag := speakerTag + vvStyleTag

			textWithEmotion := matches[3]

			// 演出用の感情タグを除去し、不要な空白を削除
			text := reEmotion.ReplaceAllString(textWithEmotion, "")
			text = strings.TrimSpace(text)

			if text != "" {
				segments = append(segments, scriptSegment{
					SpeakerTag: combinedTag, // 結合タグを格納 (フォールバック処理で再解析する)
					Text:       text,
				})
			}
		}
	}
	return segments
}

// combineWavData は複数の完全なWAVバイトデータを結合し、一つのWAVファイルを返します。
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
