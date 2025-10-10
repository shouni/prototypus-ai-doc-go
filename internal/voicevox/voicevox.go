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
	"sync"
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

// WAVヘッダーのマジックナンバーの定数化 (変更なし)
const (
	// WAV/RIFF チャンク
	RiffChunkIDSize    = 4
	RiffChunkSizeField = 4
	WaveIDSize         = 4
	WavRiffHeaderSize  = RiffChunkIDSize + RiffChunkSizeField + WaveIDSize // 12 bytes

	// FMT チャンク
	FmtChunkIDSize    = 4
	FmtChunkSizeField = 4
	FmtChunkDataSize  = 16
	WavFmtChunkSize   = FmtChunkIDSize + FmtChunkSizeField + FmtChunkDataSize // 24 bytes

	// DATA チャンク
	DataChunkIDSize    = 4
	DataChunkSizeField = 4
	WavDataHeaderSize  = DataChunkIDSize + DataChunkSizeField // 8 bytes

	// 合計ヘッダーサイズ: 12 + 24 + 8 = 44 bytes
	WavTotalHeaderSize = WavRiffHeaderSize + WavFmtChunkSize + WavDataHeaderSize
)

// ----------------------------------------------------------------------
// 並列処理用の構造体
// ----------------------------------------------------------------------

type scriptSegment struct {
	SpeakerTag string
	Text       string
}

// resultSegment は並列処理の結果を、元の順序（index）とともに保持します。
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

	// タイムアウトを設定したHTTPクライアント
	client := &http.Client{
		Timeout: 180 * time.Second,
	}

	segments := parseScript(scriptContent)
	if len(segments) == 0 {
		return fmt.Errorf("スクリプトから有効なセグメントを抽出できませんでした")
	}

	// 並列処理のためのセットアップ
	var wg sync.WaitGroup
	errChan := make(chan error, len(segments))
	resultsChan := make(chan resultSegment, len(segments))

	// ★ 同時実行数 5 のセマフォを定義
	const maxConcurrency = 5
	semaphore := make(chan struct{}, maxConcurrency)

	for i, seg := range segments {
		if seg.Text == "" {
			continue
		}

		// セマフォにトークンが書き込まれるまで待機（同時実行数を制限）
		semaphore <- struct{}{}
		wg.Add(1)

		go func(i int, seg scriptSegment) {
			defer wg.Done()
			defer func() { <-semaphore }() // 処理終了時にトークンを解放

			styleID, ok := StyleIDMappings[seg.SpeakerTag]
			if !ok {
				errChan <- fmt.Errorf("話者タグ %s に対応するVOICEVOX Style IDが見つかりません (セグメント %d)", seg.SpeakerTag, i)
				return
			}

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

			// 順序情報と結果をチャネルに送信
			resultsChan <- resultSegment{index: i, wavData: wavData}

		}(i, seg) // ループ変数をクロージャに渡す
	}

	// すべてのゴルーチンの終了を待つ
	wg.Wait()
	close(resultsChan)
	close(errChan)

	// エラーチェック
	if err := <-errChan; err != nil {
		return err // 発生した最初のエラーを返す
	}

	// 結果を元の順序で並べ替える
	// 結果を一時スライスに格納
	results := make([]resultSegment, 0, len(segments))
	for res := range resultsChan {
		results = append(results, res)
	}

	// indexに基づいてソート
	// (簡略化のため、結果スライスをセグメント数と同じサイズで初期化し、indexを使って直接格納する方がより効率的)
	if len(results) != len(segments) {
		// エラー処理などでスキップされたセグメントがある可能性を考慮
	}

	// 最終的な音声データのリストを作成
	orderedAudioDataList := make([][]byte, len(segments))
	for _, res := range results {
		// parseScriptで抽出され、かつAPIコールが成功したセグメントの結果のみを元のインデックス位置に配置
		orderedAudioDataList[res.index] = res.wavData
	}

	// APIコールがスキップされ、orderedAudioDataListの要素がnilになっているものを除外
	finalAudioDataList := make([][]byte, 0, len(results))
	for _, data := range orderedAudioDataList {
		if data != nil {
			finalAudioDataList = append(finalAudioDataList, data)
		}
	}

	if len(finalAudioDataList) == 0 {
		return fmt.Errorf("すべてのセグメントの合成に失敗したか、有効なセグメントがありませんでした")
	}

	// 3. 音声ファイルのマージと保存 (WAV連結処理)
	combinedWavBytes, err := combineWavData(finalAudioDataList)
	if err != nil {
		return fmt.Errorf("WAVデータの結合に失敗しました: %w", err)
	}

	return os.WriteFile(outputWavFile, combinedWavBytes, 0644)
}

// ----------------------------------------------------------------------
// ヘルパー関数 (API呼び出しを分離)
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

// ----------------------------------------------------------------------
// parseScript と combineWavData (変更なし)
// ----------------------------------------------------------------------

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
