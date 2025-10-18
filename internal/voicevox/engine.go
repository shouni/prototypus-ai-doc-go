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

// styleIDCache は、処理中に決定されたタグとIDのペアをキャッシュする
// キー: "[話者][スタイル]" (string), 値: Style ID (int)
var styleIDCache = make(map[string]int)

// スクリプト解析用
type scriptSegment struct {
	SpeakerTag     string // 例: "[ずんだもん][ノーマル]"
	BaseSpeakerTag string // 速度改善のために追加: 例: "[ずんだもん]" (正規表現で事前に抽出)
	Text           string
	StyleID        int   // 速度改善のために追加: 事前計算したStyle ID
	Err            error // 速度改善のために追加: 事前計算で発生したエラー
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
// ★ 修正: 内部キャッシュのチェックと更新を追加
func determineStyleID(ctx context.Context, seg scriptSegment, speakerData *SpeakerData, index int) (int, error) {
	tag := seg.SpeakerTag

	// 1. 内部キャッシュのチェック (最も高速なチェック)
	if id, ok := styleIDCache[tag]; ok {
		return id, nil
	}

	// 2. 完全なタグでの検索 (キャッシュミスの場合)
	styleID, ok := speakerData.StyleIDMap[tag]
	if ok {
		// キャッシュに保存してリターン
		styleIDCache[tag] = styleID
		return styleID, nil
	}

	// 3. フォールバック処理: デフォルトスタイルを試す
	// 事前計算されたBaseSpeakerTagを使用
	baseSpeakerTag := seg.BaseSpeakerTag

	if baseSpeakerTag == "" {
		return 0, fmt.Errorf("話者タグ %s の事前解析に失敗しました (セグメント %d)", tag, index)
	}

	fallbackKey, defaultOk := speakerData.DefaultStyleMap[baseSpeakerTag]

	slog.WarnContext(ctx, "AI出力タグが未定義のためフォールバックを試みます",
		"segment_index", index,
		"original_tag", tag,
		"fallback_key", fallbackKey)

	if defaultOk {
		// デフォルトスタイルキーに対応するIDを検索
		styleID, _ = speakerData.StyleIDMap[fallbackKey]

		// フォールバック成功の場合もキャッシュに保存
		styleIDCache[tag] = styleID

		return styleID, nil
	}

	return 0, fmt.Errorf("話者・スタイルタグ %s (およびデフォルトスタイル) に対応するStyle IDが見つかりません (セグメント %d)", tag, index)
}

// processSegment は単一のセグメントに対してAPI呼び出しを実行します。
// 修正: determineStyleIDの呼び出しを削除し、seg構造体からIDを直接取得
func processSegment(ctx context.Context, client *Client, seg scriptSegment, speakerData *SpeakerData, index int) segmentResult {
	// 1. スタイルIDの決定
	// 事前計算でエラーが発生している場合は、ここで処理を終了
	if seg.Err != nil {
		return segmentResult{index: index, err: seg.Err}
	}
	styleID := seg.StyleID // 構造体から直接取得

	var queryBody []byte
	var wavData []byte
	var currentErr error

	// 2. runAudioQuery: クライアント内部でリトライが実行される
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
func PostToEngine(ctx context.Context, scriptContent string, outputWavFile string, speakerData *SpeakerData, client *Client) error {
	segments := parseScript(scriptContent)

	if len(segments) == 0 {
		return fmt.Errorf("スクリプトから有効なセグメントを抽出できませんでした。AIの出力形式が [話者タグ][スタイルタグ] テキスト の形式に沿っているか確認してください")
	}

	// ===================================================================
	// 速度改善ステップ: 並列処理前に全セグメントのStyle IDを事前計算
	// ===================================================================
	var preCalcErrors []string
	for i := range segments {
		seg := &segments[i] // ポインターでアクセス

		// 1. 正規表現による話者タグの抽出 (Goroutine外で一度だけ実行)
		speakerMatch := reSpeaker.FindStringSubmatch(seg.SpeakerTag)
		if len(speakerMatch) >= 2 {
			seg.BaseSpeakerTag = speakerMatch[1]
		}

		// 2. Style IDの決定 (determineStyleIDはキャッシュを使用/更新する)
		styleID, err := determineStyleID(ctx, *seg, speakerData, i)
		if err != nil {
			seg.Err = err
			preCalcErrors = append(preCalcErrors, err.Error())
		} else {
			seg.StyleID = styleID
		}
	}

	// すべてのセグメントが事前計算で失敗した場合は中断
	if len(preCalcErrors) == len(segments) {
		return fmt.Errorf("すべてのセグメントのスタイルID決定に失敗しました:\n- %s", strings.Join(preCalcErrors, "\n- "))
	}
	// ===================================================================

	var wg sync.WaitGroup
	// resultsChanで正常な結果とエラーの両方を集約
	resultsChan := make(chan segmentResult, len(segments))

	semaphore := make(chan struct{}, maxParallelSegments)

	// ===================================================================
	// セグメントごとの並列処理開始 (事前計算された情報を使用)
	// ===================================================================
	for i, seg := range segments {
		// 事前計算でテキストがない、または失敗したセグメントは並列処理を行わない
		if seg.Text == "" || seg.Err != nil {
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
	var runtimeErrors []string

	// 事前計算で発生したエラーも集約リストに追加する
	allErrors := append([]string{}, preCalcErrors...)

	for res := range resultsChan {
		if res.err != nil {
			// 実行時エラーを収集
			runtimeErrors = append(runtimeErrors, res.err.Error())
		} else if res.wavData != nil {
			// 正常なデータをインデックス位置に格納
			if res.index >= 0 && res.index < len(segments) {
				orderedAudioDataList[res.index] = res.wavData
			}
		}
	}

	// 実行時エラーを全体のリストに追加
	allErrors = append(allErrors, runtimeErrors...)

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
