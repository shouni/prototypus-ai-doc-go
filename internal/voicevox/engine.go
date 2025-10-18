package voicevox

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ----------------------------------------------------------------------
// 定数・構造体
// ----------------------------------------------------------------------

const (
	maxParallelSegments = 15                // 同時実行セグメントの最大数
	segmentTimeout      = 120 * time.Second // 1セグメントの処理に最大120秒を許容
)

var reSpeaker = regexp.MustCompile(`^(\[.+?\])`)

// スクリプト解析用
type scriptSegment struct {
	SpeakerTag string // 例: "[ずんだもん][ノーマル]"
	Text       string
}

// Goroutineの結果を格納
type segmentResult struct {
	index   int
	wavData []byte
	err     error
}

// ----------------------------------------------------------------------
// ヘルパー関数
// ----------------------------------------------------------------------

// determineStyleID はセグメントの話者タグから対応するStyle IDを検索します。
// 見つからない場合はフォールバック処理を試みます。
// ※ この関数は変更なし
func determineStyleID(ctx context.Context, seg scriptSegment, speakerData *SpeakerData, index int) (int, error) {
	// 1. 完全なタグでの検索
	styleID, ok := speakerData.StyleIDMap[seg.SpeakerTag]
	if ok {
		return styleID, nil
	}

	// 2. フォールバック処理: デフォルトスタイルを試す
	speakerMatch := reSpeaker.FindStringSubmatch(seg.SpeakerTag)

	if len(speakerMatch) < 2 {
		return 0, fmt.Errorf("話者タグ %s の解析に失敗しました (セグメント %d)", seg.SpeakerTag, index)
	}

	baseSpeakerTag := speakerMatch[1]
	fallbackKey, defaultOk := speakerData.DefaultStyleMap[baseSpeakerTag]

	slog.WarnContext(ctx, "AI出力タグが未定義のためフォールバックを試みます",
		"segment_index", index,
		"original_tag", seg.SpeakerTag,
		"fallback_key", fallbackKey)

	if defaultOk {
		// デフォルトスタイルキーに対応するIDを検索 (LoadSpeakersで存在確認済み)
		styleID, _ = speakerData.StyleIDMap[fallbackKey]
		return styleID, nil
	}

	return 0, fmt.Errorf("話者・スタイルタグ %s (およびデフォルトスタイル) に対応するStyle IDが見つかりません (セグメント %d)", seg.SpeakerTag, index)
}

// processSegment は単一のセグメントに対してAPI呼び出しを実行します。
func processSegment(ctx context.Context, client *Client, seg scriptSegment, speakerData *SpeakerData, index int) segmentResult {
	// 1. スタイルIDの決定
	styleID, err := determineStyleID(ctx, seg, speakerData, index)
	if err != nil {
		return segmentResult{index: index, err: err}
	}

	var queryBody []byte
	var wavData []byte
	var currentErr error

	// 2. runAudioQuery: クライアント内部でリトライが実行される
	// processSegmentはリトライの成否を心配せず、結果だけを受け取る
	queryBody, currentErr = client.runAudioQuery(seg.Text, styleID, ctx)
	if currentErr != nil {
		return segmentResult{index: index, err: fmt.Errorf("セグメント %d のオーディオクエリ失敗: %w", index, currentErr)}
	}

	// 3. runSynthesis: クライアント内部でリトライが実行される
	wavData, currentErr = client.runSynthesis(queryBody, styleID, ctx)
	if currentErr != nil {
		return segmentResult{index: index, err: fmt.Errorf("セグメント %d の音声合成失敗: %w", index, currentErr)}
	}

	// 4. 成功
	return segmentResult{index: index, wavData: wavData}
}

// ----------------------------------------------------------------------
// メイン処理 (PostToEngine)
// ----------------------------------------------------------------------

// PostToEngine はスクリプト全体をVOICEVOXエンジンに投稿し、音声ファイルを生成するメイン関数です。
// 外部からClientインスタンスを受け取ることで、DI（依存性注入）に対応します。
func PostToEngine(ctx context.Context, scriptContent string, outputWavFile string, speakerData *SpeakerData, client *Client) error {
	// 以前: client := NewClient(apiURL) -> 外部から渡されるため削除

	segments := parseScript(scriptContent)

	if len(segments) == 0 {
		return fmt.Errorf("スクリプトから有効なセグメントを抽出できませんでした。AIの出力形式が [話者タグ][スタイルタグ] テキスト の形式に沿っているか確認してください")
	}

	var wg sync.WaitGroup
	// resultsChanで正常な結果とエラーの両方を集約
	resultsChan := make(chan segmentResult, len(segments))

	semaphore := make(chan struct{}, maxParallelSegments)

	// ===================================================================
	// セグメントごとの並列処理開始
	// ===================================================================
	for i, seg := range segments {
		if seg.Text == "" {
			continue
		}

		semaphore <- struct{}{} // セマフォ取得 (ブロックされる可能性あり)
		wg.Add(1)

		go func(i int, seg scriptSegment) {
			defer wg.Done()
			defer func() { <-semaphore }() // セマフォ解放

			// セグメントごとのコンテキストタイムアウトを設定
			segCtx, cancel := context.WithTimeout(ctx, segmentTimeout)
			defer cancel()

			// 実際の処理は processSegment に委譲
			result := processSegment(segCtx, client, seg, speakerData, i)
			resultsChan <- result

		}(i, seg)
	}
	// ===================================================================
	// 並列処理終了後の集約
	// ===================================================================

	wg.Wait()
	close(resultsChan)

	// 順序の再構築とエラーの集約
	orderedAudioDataList := make([][]byte, len(segments))
	var allErrors []string

	for res := range resultsChan {
		if res.err != nil {
			// エラーを収集
			allErrors = append(allErrors, res.err.Error())
		} else if res.wavData != nil {
			// 正常なデータをインデックス位置に格納
			if res.index >= 0 && res.index < len(segments) {
				orderedAudioDataList[res.index] = res.wavData
			}
		}
	}

	if len(allErrors) > 0 {
		// エラーが発生した場合、すべてのエラーを結合して返す
		return fmt.Errorf("音声合成処理中に %d 件のエラーが発生しました:\n- %s", len(allErrors), strings.Join(allErrors, "\n- "))
	}

	// 最終的なWAVデータリストの構築 (nilをスキップ)
	finalAudioDataList := make([][]byte, 0, len(orderedAudioDataList))
	for _, data := range orderedAudioDataList {
		if data != nil {
			finalAudioDataList = append(finalAudioDataList, data)
		}
	}

	if len(finalAudioDataList) == 0 {
		return fmt.Errorf("すべてのセグメントの合成に失敗したか、有効なセグメントがありませんでした")
	}

	// 責務の分離: WAVデータの結合
	combinedWavBytes, err := combineWavData(finalAudioDataList)
	if err != nil {
		return fmt.Errorf("WAVデータの結合に失敗しました: %w", err)
	}

	slog.InfoContext(ctx, "全てのセグメントの合成と結合が完了しました。ファイル書き込みを行います。", "output_file", outputWavFile)

	return os.WriteFile(outputWavFile, combinedWavBytes, 0644)
}
