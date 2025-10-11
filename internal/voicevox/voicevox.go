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

const (
	SpeakerTagZundamon = "[ずんだもん]"
	SpeakerTagMetan    = "[めたん]"
)

const (
	VvTagNormal   = "[通常]"
	VvTagHappy    = "[喜び]"
	VvTagAngry    = "[怒り]"
	VvTagWhisper  = "[ささやき]"
	VvTagGrasping = "[納得]"
)

// StyleIDMappingsに [ずんだもん][納得] を追加（IDを20から35に変更）
var StyleIDMappings = map[string]int{
	SpeakerTagZundamon + VvTagNormal:   3,
	SpeakerTagZundamon + VvTagHappy:    1,
	SpeakerTagZundamon + VvTagAngry:    4,
	SpeakerTagZundamon + VvTagWhisper:  18,
	SpeakerTagZundamon + VvTagGrasping: 35, // ★ 2. 仮のStyle IDをより確度の高い35に変更
	SpeakerTagMetan + VvTagNormal:      2,
	SpeakerTagMetan + VvTagHappy:       15,
	SpeakerTagMetan + VvTagAngry:       17,
}

// 許可された話者タグを定義 (parseScriptで使用)
var AllowedSpeakerTags = map[string]bool{
	SpeakerTagZundamon: true,
	SpeakerTagMetan:    true,
}

// ----------------------------------------------------------------------
// 定数 (WAVヘッダー)
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
	SpeakerTag string // 例: "[ずんだもん][通常]"
	Text       string
}

type resultSegment struct {
	index   int
	wavData []byte
}

// ----------------------------------------------------------------------
// メイン処理 (堅牢性向上)
// ----------------------------------------------------------------------

// PostToEngine はスクリプト全体をVOICEVOXエンジンに投稿し、音声ファイルを生成します。
func PostToEngine(scriptContent string, outputWavFile string) error {
	apiURL := os.Getenv("VOICEVOX_API_URL")
	if apiURL == "" {
		return fmt.Errorf("VOICEVOX_API_URL 環境変数が設定されていません")
	}

	client := &http.Client{
		Timeout: 90 * time.Second,
	}

	segments := parseScript(scriptContent)
	if len(segments) == 0 {
		return fmt.Errorf("スクリプトから有効なセグメントを抽出できませんでした。AIの出力形式が [話者タグ][スタイルタグ] テキスト の形式に沿っているか確認してください")
	}

	var wg sync.WaitGroup
	// ★ 1. 修正: errChanのバッファサイズを len(segments) に戻し、全てのエラーをキャプチャ可能にする
	errChan := make(chan error, len(segments))
	resultsChan := make(chan resultSegment, len(segments))

	// 並列実行数を15に設定
	const maxConcurrency = 15
	semaphore := make(chan struct{}, maxConcurrency)

	// リトライ設定の定義
	const maxRetries = 3
	const retryDelay = 2 * time.Second

	for i, seg := range segments {
		if seg.Text == "" {
			continue
		}

		semaphore <- struct{}{}
		wg.Add(1)

		go func(i int, seg scriptSegment) {
			defer wg.Done()
			defer func() { <-semaphore }()

			// 失敗時にerrChanにエラーを送信するヘルパー関数
			sendError := func(err error) {
				// selectを使用して非ブロッキングでエラーを送信。チャネルが満杯でない限り全てのエラーをキャプチャする。
				select {
				case errChan <- err:
				default:
					// errChanが満杯（非常に大量のエラーが発生）の場合、このエラーは無視する
				}
			}

			// スタイルIDの検索とフォールバック処理
			styleID, ok := StyleIDMappings[seg.SpeakerTag]

			if !ok {
				reSpeaker := regexp.MustCompile(`^(\[.+?\])`)
				speakerMatch := reSpeaker.FindStringSubmatch(seg.SpeakerTag)

				if len(speakerMatch) < 2 {
					sendError(fmt.Errorf("話者タグ %s の解析に失敗しました (セグメント %d)", seg.SpeakerTag, i))
					return
				}

				baseSpeakerTag := speakerMatch[1]
				fallbackKey := baseSpeakerTag + VvTagNormal

				// ログメッセージ
				fmt.Printf("警告: スタイルタグ %s がStyleIDMappingsに見つかりません。デフォルトの %s へフォールバックを試みます (セグメント %d)\n", seg.SpeakerTag, fallbackKey, i)

				defaultStyleID, defaultOk := StyleIDMappings[fallbackKey]

				if defaultOk {
					styleID = defaultStyleID
					fmt.Printf("警告: フォールバック成功。Style ID: %d を使用します (セグメント %d)\n", styleID, i)
				} else {
					sendError(fmt.Errorf("話者・スタイルタグ %s (およびデフォルトの %s) に対応するVOICEVOX Style IDが見つかりません (セグメント %d)", seg.SpeakerTag, fallbackKey, i))
					return
				}
			}

			// リトライロジックの追加
			var queryBody []byte
			var wavData []byte
			var currentErr error

			// 処理が成功するまで最大 maxRetries 回試行
			for attempt := 1; attempt <= maxRetries; attempt++ {
				// 1. オーディオクエリ (audio_query) の作成
				queryBody, currentErr = runAudioQuery(client, apiURL, seg.Text, styleID)
				if currentErr != nil {
					// 最終試行でなければリトライ
					if attempt < maxRetries {
						// エラーメッセージのログ出力（テキストが長すぎないように制御）
						textSnippet := seg.Text
						if len(textSnippet) > 20 {
							textSnippet = textSnippet[:20] + "..."
						}
						fmt.Printf("警告: セグメント %d (テキスト: \"%s\") のオーディオクエリでエラー。%d/%d 回目のリトライを %v 後に実行します: %v\n", i, textSnippet, attempt, maxRetries, retryDelay, currentErr)
						time.Sleep(retryDelay)
						continue
					}
					// 最終試行で失敗
					sendError(fmt.Errorf("セグメント %d のオーディオクエリが連続失敗: %w", i, currentErr))
					return
				}

				// 2. 音声合成 (synthesis) の実行
				wavData, currentErr = runSynthesis(client, apiURL, queryBody, styleID)
				if currentErr != nil {
					// 最終試行でなければリトライ
					if attempt < maxRetries {
						textSnippet := seg.Text
						if len(textSnippet) > 20 {
							textSnippet = textSnippet[:20] + "..."
						}
						fmt.Printf("警告: セグメント %d (テキスト: \"%s\") の音声合成でエラー。%d/%d 回目のリトライを %v 後に実行します: %v\n", i, textSnippet, attempt, maxRetries, retryDelay, currentErr)
						time.Sleep(retryDelay)
						continue
					}
					// 最終試行で失敗
					sendError(fmt.Errorf("セグメント %d の音声合成が連続失敗: %w", i, currentErr))
					return
				}

				// 両方の処理が成功したらループを抜ける
				break
			}

			resultsChan <- resultSegment{index: i, wavData: wavData}

		}(i, seg)
	}

	wg.Wait()
	close(resultsChan)
	close(errChan)

	// ★ 1. 修正: errChanから全てのエラーを読み取り、一つの詳細なエラーメッセージに結合する
	var allErrors []string
	for err := range errChan {
		if err != nil {
			allErrors = append(allErrors, err.Error())
		}
	}

	if len(allErrors) > 0 {
		// 発生した全てのエラーをリスト化して返す
		return fmt.Errorf("音声合成処理中に %d 件のエラーが発生しました:\n- %s", len(allErrors), strings.Join(allErrors, "\n- "))
	}

	// 後続のWAV結合処理は変更なし
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

func parseScript(script string) []scriptSegment {
	// 最初の2つのタグを抽出する正規表現（例: [ずんだもん][通常]）
	re := regexp.MustCompile(`^(\[.+?\])\s*(\[.+?\])\s*(.*)`)

	// 感情タグを除去するための正規表現
	// [納得] は、話者スタイルタグとして使用されるようになったため、除去対象から除外
	reEmotion := regexp.MustCompile(
		`\[(解説|疑問|驚き|理解|落ち着き|断定|呼びかけ)\]`,
	)

	lines := bytes.Split([]byte(script), []byte("\n"))
	var segments []scriptSegment

	for _, lineBytes := range lines {
		line := string(bytes.TrimSpace(lineBytes))
		if line == "" {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) > 3 {
			speakerTag := matches[1] // 例: "[ずんだもん]"
			vvStyleTag := matches[2] // 例: "[通常]"

			// 許可された話者タグかどうかをチェック (不正なタグの混入対策)
			if !AllowedSpeakerTags[speakerTag] {
				// ログ出力で不正なタグの存在を警告し、このセグメントをスキップ
				fmt.Printf("警告: スクリプトの不正な話者タグをスキップします: %s (行: %s)\n", speakerTag, line)
				continue
			}

			combinedTag := speakerTag + vvStyleTag // 例: "[ずんだもん][通常]"

			textWithEmotion := matches[3]

			// 修正後のreEmotionを適用し、[納得] の意図しない除去を回避
			text := reEmotion.ReplaceAllString(textWithEmotion, "")
			text = strings.TrimSpace(text)

			if text != "" {
				segments = append(segments, scriptSegment{
					SpeakerTag: combinedTag,
					// 修正: 感情タグを除去したテキストを使用
					Text: text,
				})
			}
		}
	}
	return segments
}

func combineWavData(wavFiles [][]byte) ([]byte, error) {
	var rawData []byte
	totalDataSize := uint32(0)

	fmtChunkEndIndex := WavRiffHeaderSize + WavFmtChunkSize
	if len(wavFiles[0]) < fmtChunkEndIndex {
		return nil, fmt.Errorf("最初のWAVファイルのヘッダー（RIFF + FMT）が短すぎます")
	}
	formatHeader := wavFiles[0][0:fmtChunkEndIndex]

	for _, wavBytes := range wavFiles {
		if len(wavBytes) < WavTotalHeaderSize {
			return nil, fmt.Errorf("WAVファイルが完全なヘッダーを含んでいません")
		}

		dataSizeStartIndex := WavTotalHeaderSize - DataChunkSizeField
		dataSize := binary.LittleEndian.Uint32(wavBytes[dataSizeStartIndex:WavTotalHeaderSize])

		dataChunk := wavBytes[WavTotalHeaderSize : WavTotalHeaderSize+dataSize]

		rawData = append(rawData, dataChunk...)
		totalDataSize += dataSize
	}

	combinedWav := make([]byte, WavTotalHeaderSize+totalDataSize)
	copy(combinedWav, formatHeader)

	dataIDStartIndex := WavRiffHeaderSize + WavFmtChunkSize
	copy(combinedWav[dataIDStartIndex:], []byte("data"))

	dataSizeStartIndex := WavTotalHeaderSize - DataChunkSizeField
	binary.LittleEndian.PutUint32(combinedWav[dataSizeStartIndex:WavTotalHeaderSize], totalDataSize)

	fileSize := WavTotalHeaderSize + totalDataSize - WavRiffHeaderSize + RiffChunkSizeField
	fileSizeStartIndex := RiffChunkIDSize
	binary.LittleEndian.PutUint32(combinedWav[fileSizeStartIndex:fileSizeStartIndex+RiffChunkSizeField], fileSize)

	copy(combinedWav[WavTotalHeaderSize:], rawData)

	return combinedWav, nil
}
