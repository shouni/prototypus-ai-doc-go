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

// 並列処理用の構造体
type scriptSegment struct {
	SpeakerTag string // 例: "[ずんだもん][ノーマル]"
	Text       string
}

type resultSegment struct {
	index   int
	wavData []byte
}

const (
	maxParallelSegments = 15                // 同時実行セグメントの最大数
	maxRetries          = 3                 // API呼び出しのリトライ回数
	segmentTimeout      = 120 * time.Second // 1セグメントの処理に最大120秒を許容
)

// ----------------------------------------------------------------------
// メイン処理 (PostToEngine)
// ----------------------------------------------------------------------

// PostToEngine はスクリプト全体をVOICEVOXエンジンに投稿し、音声ファイルを生成するメイン関数です。
// この関数は並列処理、リトライロジック、エラー集約を制御します。
func PostToEngine(ctx context.Context, scriptContent string, outputWavFile string, speakerData *SpeakerData, apiURL string) error {

	client := NewAPIClient(apiURL)
	segments := parseScript(scriptContent)
	if len(segments) == 0 {
		return fmt.Errorf("スクリプトから有効なセグメントを抽出できませんでした。AIの出力形式が [話者タグ][スタイルタグ] テキスト の形式に沿っているか確認してください")
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(segments))
	resultsChan := make(chan resultSegment, len(segments))

	// ★ 修正 1: maxParallelSegments を使用 (ローカル定数 maxConcurrency を削除)
	semaphore := make(chan struct{}, maxParallelSegments)

	// ★ 修正 2: maxRetries のローカル定義を削除。retryDelay は残す。
	const retryDelay = 2 * time.Second

	// ===================================================================
	// セグメントごとの並列処理開始
	// ===================================================================
	for i, seg := range segments {
		if seg.Text == "" {
			continue
		}

		// グローバルなコンテキストキャンセルをチェック
		select {
		case <-ctx.Done():
			slog.Info("処理がキャンセルされました（グローバルコンテキスト）")
			return ctx.Err()
		default:
		}

		semaphore <- struct{}{}
		wg.Add(1)

		go func(i int, seg scriptSegment) {
			defer wg.Done()
			defer func() { <-semaphore }()

			segCtx, cancel := context.WithTimeout(ctx, segmentTimeout)
			defer cancel()

			// Goroutine開始直後のコンテキストキャンセルチェック
			if ctx.Err() != nil {
				return
			}

			// ★ 修正 3: sendError をここで定義
			// 非ブロッキングでエラーを送信するヘルパー関数
			sendError := func(err error) {
				select {
				case errChan <- err:
				default:
				}
			}

			// ★ 修正 4: styleID をここで定義
			var styleID int
			var ok bool

			// 1. スタイルIDの動的な検索とフォールバック処理
			styleID, ok = speakerData.StyleIDMap[seg.SpeakerTag]
			if !ok {
				// 話者タグのみを抽出（例: [ずんだもん]）
				reSpeaker := regexp.MustCompile(`^(\[.+?\])`)
				speakerMatch := reSpeaker.FindStringSubmatch(seg.SpeakerTag)

				if len(speakerMatch) < 2 {
					sendError(fmt.Errorf("話者タグ %s の解析に失敗しました (セグメント %d)", seg.SpeakerTag, i))
					return
				}

				baseSpeakerTag := speakerMatch[1]
				fallbackKey, defaultOk := speakerData.DefaultStyleMap[baseSpeakerTag]

				slog.WarnContext(ctx, "AI出力タグが未定義のためフォールバックを試みます",
					"segment_index", i,
					"original_tag", seg.SpeakerTag,
					"fallback_key", fallbackKey)

				if defaultOk {
					// ★ styleID はすでに定義されているため、ここでは 'styleID, _ =' ではなく 'styleID =' で代入
					styleID, _ = speakerData.StyleIDMap[fallbackKey] // 存在することはLoadSpeakersで確認済み
				} else {
					sendError(fmt.Errorf("話者・スタイルタグ %s (およびデフォルトスタイル) に対応するStyle IDが見つかりません (セグメント %d)", seg.SpeakerTag, i))
					return
				}
			}

			// 2. リトライロジックの実行
			var queryBody []byte
			var wavData []byte
			var currentErr error
			var success bool // 成功フラグの追加

			for attempt := 1; attempt <= maxRetries; attempt++ {

				// コンテキストのキャンセルをチェック
				if segCtx.Err() != nil {
					slog.Info("処理がキャンセルされました...", "segment_index", i)
					return
				}

				// API呼び出し (query と synthesis) を実行
				queryBody, currentErr = client.runAudioQuery(seg.Text, styleID, segCtx)
				if currentErr == nil {
					wavData, currentErr = client.runSynthesis(queryBody, styleID, segCtx)
				}

				// 成功した場合
				if currentErr == nil {
					success = true
					break // 成功したのでループを抜ける
				}

				// 失敗した場合のリトライ判定 (goto の代替ロジック)
				if attempt < maxRetries {
					textSnippet := seg.Text
					if len(textSnippet) > 20 {
						textSnippet = textSnippet[:20] + "..."
					}

					// 指数バックオフの計算: baseDelay (2s) * 2^(attempt-1)
					backoffDelay := retryDelay * time.Duration(1<<(attempt-1))

					slog.WarnContext(ctx, "APIリクエストエラー。リトライします",
						"segment_index", i,
						"text", textSnippet,
						"attempt", attempt,
						"max_retries", maxRetries,
						"error", currentErr,
						"delay", backoffDelay) // 遅延時間をログに追加

					time.Sleep(backoffDelay)
					continue // 次の試行へ
				}

				// 最終試行で失敗
				sendError(fmt.Errorf("セグメント %d のAPIリクエストが連続失敗: %w", i, currentErr))
				return // 最終的に失敗したのでGoroutineを終了
			}

			// 成功しなかった場合は、すでに sendError が呼ばれているため return
			if !success {
				return
			}

			resultsChan <- resultSegment{index: i, wavData: wavData}

		}(i, seg)
	}
	// ===================================================================
	// 並列処理終了後の集約
	// ===================================================================

	wg.Wait()
	close(resultsChan)
	close(errChan)

	// エラーの集約
	var allErrors []string
	for err := range errChan {
		if err != nil {
			allErrors = append(allErrors, err.Error())
		}
	}

	if len(allErrors) > 0 {
		return fmt.Errorf("音声合成処理中に %d 件のエラーが発生しました:\n- %s", len(allErrors), strings.Join(allErrors, "\n- "))
	}

	// 順序の再構築と有効なデータのフィルタリング
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

	// 責務の分離: WAVデータの結合
	combinedWavBytes, err := combineWavData(finalAudioDataList)
	if err != nil {
		return fmt.Errorf("WAVデータの結合に失敗しました: %w", err)
	}

	return os.WriteFile(outputWavFile, combinedWavBytes, 0644)
}
