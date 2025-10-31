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
	maxParallelSegments = 6
	segmentTimeout      = 300 * time.Second
)

var reSpeaker = regexp.MustCompile(`^(\[.+?\])`)

// styleIDCache は、処理中に決定されたタグとIDのペアをキャッシュする
// キー: "[話者][スタイル]" (string), 値: Style ID (int)
var styleIDCache = make(map[string]int)

// styleIDCacheへの並行アクセスを保護するためのMutex
var styleIDCacheMutex sync.RWMutex

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
func determineStyleID(ctx context.Context, seg scriptSegment, speakerData *SpeakerData, index int) (int, error) {
	tag := seg.SpeakerTag

	// 1. 内部キャッシュのチェック (読み取り操作)
	styleIDCacheMutex.RLock()
	if id, ok := styleIDCache[tag]; ok {
		styleIDCacheMutex.RUnlock()
		return id, nil
	}
	styleIDCacheMutex.RUnlock()

	// 2. 完全なタグでの検索 (キャッシュミスの場合)
	styleID, ok := speakerData.StyleIDMap[tag]
	if ok {
		// キャッシュに保存 (書き込み操作)
		styleIDCacheMutex.Lock()
		styleIDCache[tag] = styleID
		styleIDCacheMutex.Unlock()
		return styleID, nil
	}

	// 3. フォールバック処理: デフォルトスタイルを試す
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

		// フォールバック成功の場合もキャッシュに保存 (書き込み操作)
		styleIDCacheMutex.Lock()
		styleIDCache[tag] = styleID
		styleIDCacheMutex.Unlock()

		return styleID, nil
	}

	return 0, fmt.Errorf("話者・スタイルタグ %s (およびデフォルトスタイル) に対応するStyle IDが見つかりません (セグメント %d)", tag, index)
}

// processSegment は単一のセグメントに対してAPI呼び出しを実行します。
func processSegment(ctx context.Context, client *Client, seg scriptSegment, index int) segmentResult {
	// 1. スタイルIDの決定
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

	if len(queryBody) == 0 {
		// /audio_query が成功しても、テキスト処理の問題で空のボディが返る可能性を考慮
		return segmentResult{index: index, err: fmt.Errorf("セグメント %d のオーディオクエリ結果が空です。入力テキストやAPI応答を確認してください", index)}
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
// NOTE: parseScript, combineWavData, SpeakerData, Client型は外部ファイルで定義されていると仮定。
func PostToEngine(ctx context.Context, scriptContent string, outputWavFile string, speakerData *SpeakerData, client *Client, fallbackTag string) error {

	// ★ 修正: fallbackTagをparseScriptに渡す
	segments := parseScript(scriptContent, fallbackTag)

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
	resultsChan := make(chan segmentResult, len(segments))

	semaphore := make(chan struct{}, maxParallelSegments)

	// ===================================================================
	// セグメントごとの並列処理開始 (事前計算された情報を使用)
	// ===================================================================
	for i, seg := range segments {
		if seg.Text == "" || seg.Err != nil {
			continue
		}

		semaphore <- struct{}{}
		wg.Add(1)

		go func(i int, seg scriptSegment) {
			defer wg.Done()
			defer func() { <-semaphore }()

			segCtx, cancel := context.WithTimeout(ctx, segmentTimeout)
			defer cancel()

			result := processSegment(segCtx, client, seg, i)
			resultsChan <- result

		}(i, seg)
	}
	// ===================================================================
	// 並列処理終了後の集約
	// ===================================================================

	wg.Wait()
	close(resultsChan)

	orderedAudioDataList := make([][]byte, len(segments))
	var runtimeErrors []string

	allErrors := append([]string{}, preCalcErrors...)

	for res := range resultsChan {
		if res.err != nil {
			runtimeErrors = append(runtimeErrors, res.err.Error())
		} else if res.wavData != nil {
			if res.index >= 0 && res.index < len(segments) {
				orderedAudioDataList[res.index] = res.wavData
			}
		}
	}

	allErrors = append(allErrors, runtimeErrors...)

	if len(allErrors) > 0 {
		return fmt.Errorf("音声合成処理中に %d 件のエラーが発生しました:\n- %s", len(allErrors), strings.Join(allErrors, "\n- "))
	}

	finalAudioDataList := make([][]byte, 0, len(orderedAudioDataList))
	for _, data := range orderedAudioDataList {
		if data != nil {
			finalAudioDataList = append(finalAudioDataList, data)
		}
	}

	if len(finalAudioDataList) == 0 {
		return fmt.Errorf("すべてのセグメントの合成に失敗したか、有効なセグメントがありませんでした")
	}

	// NOTE: combineWavDataはここでは定義されていない外部関数を想定
	combinedWavBytes, err := combineWavData(finalAudioDataList)
	if err != nil {
		return fmt.Errorf("WAVデータの結合に失敗しました: %w", err)
	}

	slog.InfoContext(ctx, "全てのセグメントの合成と結合が完了しました。ファイル書き込みを行います。", "output_file", outputWavFile)

	return os.WriteFile(outputWavFile, combinedWavBytes, 0644)
}
