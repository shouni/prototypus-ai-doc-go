package voicevox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shouni/go-web-exact/pkg/retry"
)

// ----------------------------------------------------------------------
// 定数と共通エラー型
// ----------------------------------------------------------------------

const (
	// HTTPクライアントのタイムアウトを一元管理
	httpClientTimeout = 120 * time.Second // 120秒

	// サイトからのブロックを避けるためのUser-Agent
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36"

	// MaxResponseBodySize は、あらゆるHTTPレスポンスボディの最大読み込みサイズ (10MB)
	MaxResponseBodySize = int64(10 * 1024 * 1024)
)

// NonRetryableHTTPError はHTTP 4xx系のステータスコードエラーを示すカスタムエラー型です。
type NonRetryableHTTPError struct {
	StatusCode int
	Body       []byte
}

func (e *NonRetryableHTTPError) Error() string {
	if len(e.Body) > 0 {
		const maxBodyDisplaySize = 1024
		displayBody := strings.TrimSpace(string(e.Body))
		if len(displayBody) > maxBodyDisplaySize {
			runes := []rune(displayBody)
			if len(runes) > maxBodyDisplaySize {
				displayBody = string(runes[:maxBodyDisplaySize]) + "..."
			}
		}
		return fmt.Sprintf("HTTPクライアントエラー (非リトライ対象): ステータスコード %d, ボディ: %s", e.StatusCode, displayBody)
	}
	return fmt.Sprintf("HTTPクライアントエラー (非リトライ対象): ステータスコード %d, ボディなし", e.StatusCode)
}

// IsNonRetryableError は与えられたエラーが非リトライ対象のHTTPエラーであるかを判断します。
func IsNonRetryableError(err error) bool {
	if err == nil {
		return false
	}
	var nonRetryable *NonRetryableHTTPError
	return errors.As(err, &nonRetryable)
}

// ----------------------------------------------------------------------
// Requester インターフェース (HTTP実行の抽象化)
// ----------------------------------------------------------------------

// Requester は、HTTPリクエストの実行とエラー処理、リトライを抽象化するインターフェースです。
// Clientはこのインターフェースに依存することで、net/httpの詳細から分離されます。
type Requester interface {
	DoRequest(ctx context.Context, method, fullURL string, body io.Reader, headers map[string]string) ([]byte, error)
	Get(url string, ctx context.Context) (*http.Response, error) // 汎用GET (リトライなし)
}

// ----------------------------------------------------------------------
// RetryHTTPRequester (Requesterの具象実装: HTTP接続とリトライの関心)
// ----------------------------------------------------------------------

// RetryHTTPRequester はRequesterインターフェースの実装で、
// 実際のHTTPリクエスト実行、リトライ、共通エラー処理の関心を受け持ちます。
type RetryHTTPRequester struct {
	httpClient  *http.Client
	retryConfig retry.Config
}

// NewRetryHTTPRequester は新しい RetryHTTPRequester を初期化します。
func NewRetryHTTPRequester(timeout time.Duration) *RetryHTTPRequester {
	return &RetryHTTPRequester{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retryConfig: retry.DefaultConfig(),
	}
}

// addCommonHeaders は共通のHTTPヘッダーを設定します。
func (r *RetryHTTPRequester) addCommonHeaders(req *http.Request) {
	req.Header.Set("User-Agent", UserAgent)
}

// isHTTPRetryableError はエラーがHTTPリトライ対象かどうかを判定します。
func (r *RetryHTTPRequester) isHTTPRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// 1. Contextエラー（タイムアウト/キャンセル）はリトライ対象
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// 2. 非リトライ対象エラー（4xx）はリトライしない
	if IsNonRetryableError(err) {
		return false
	}
	// 3. 5xxエラーやネットワークエラー（NonRetryableHTTPErrorでないもの）はすべてリトライ対象
	return true
}

// doWithRetry は共通のリトライロジックを実行します。
func (r *RetryHTTPRequester) doWithRetry(ctx context.Context, operationName string, op func() error) error {
	return retry.Do(
		ctx,
		r.retryConfig,
		operationName,
		op,
		r.isHTTPRetryableError,
	)
}

// handleResponseはHTTPレスポンスを処理し、成功した場合はボディをバイト配列として返します。
func (r *RetryHTTPRequester) handleResponse(resp *http.Response) ([]byte, error) {
	// deferは最初に実行し、エラーが発生した場合でも必ずBodyを閉じるようにします。
	defer resp.Body.Close()

	// ContentLengthによる事前チェック
	if resp.ContentLength > 0 && resp.ContentLength > MaxResponseBodySize {
		return nil, fmt.Errorf("レスポンスボディが最大サイズ (%dバイト) を超えました (Content-Length ヘッダーによるチェック)", MaxResponseBodySize)
	}

	// MaxResponseBodySize + 1 バイトを制限に設定し、制限超過を検出できるようにします。
	limitedReader := io.LimitReader(resp.Body, MaxResponseBodySize+1)

	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("レスポンスボディの読み込みに失敗しました: %w", err)
	}

	// 実際に読み込んだバイト数が MaxResponseBodySize を超えていた場合、エラーを返します。
	if len(bodyBytes) > int(MaxResponseBodySize) {
		return nil, fmt.Errorf("レスポンスボディが最大サイズ (%dバイト) を超えました (読み込みサイズ: %d)", MaxResponseBodySize, MaxResponseBodySize)
	}

	// 2xx系は成功
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return bodyBytes, nil
	}

	// 5xx 系: リトライ対象のサーバーエラー
	if resp.StatusCode >= 500 && resp.StatusCode <= 599 {
		// リトライ対象としてエラーを返す
		return nil, fmt.Errorf("HTTPステータスコードエラー (5xx リトライ対象): %d, 詳細: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	// 4xx 系など、その他は非リトライ対象のクライアントエラー
	return nil, &NonRetryableHTTPError{
		StatusCode: resp.StatusCode,
		Body:       bodyBytes,
	}
}

// DoRequest は Requester インターフェースを実装します。リクエストの作成、リトライ、エラー判定の全てをここで実行します。
func (r *RetryHTTPRequester) DoRequest(ctx context.Context, method, fullURL string, body io.Reader, headers map[string]string) ([]byte, error) {
	var bodyBytes []byte

	// リトライロジックをラップ
	op := func() error {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
		if err != nil {
			return fmt.Errorf("HTTPリクエスト作成失敗: %w", err)
		}

		// 共通ヘッダー（User-Agent）とカスタムヘッダーを追加
		r.addCommonHeaders(req)
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := r.httpClient.Do(req)
		if err != nil {
			// ネットワークエラー、Contextエラーなど
			return fmt.Errorf("HTTPリクエスト実行失敗: %w", err)
		}

		// handleResponseでエラーとリトライ可否を判定
		bodyBytes, err = r.handleResponse(resp)
		return err
	}

	err := r.doWithRetry(
		ctx,
		fmt.Sprintf("%s %sの実行", method, fullURL),
		op,
	)

	if err != nil {
		return nil, err
	}

	return bodyBytes, nil
}

// Get は汎用のGETリクエストを実行します。（リトライなし）
func (r *RetryHTTPRequester) Get(url string, ctx context.Context) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	r.addCommonHeaders(req) // User-Agentを追加
	// contextを付与したリクエストを実行
	return r.httpClient.Do(req)
}

// ----------------------------------------------------------------------
// Client 構造体 (VOICEVOX特有のロジックの関心)
// ----------------------------------------------------------------------

// Client はVOICEVOXエンジンへのAPIリクエストを処理するクライアントです。
type Client struct {
	requester Requester // Requesterインターフェースに依存
	apiURL    string
}

// NewClient は新しいClientインスタンスを初期化します。
// 内部で RetryHTTPRequester のインスタンスを生成し、依存性を注入します。
func NewClient(apiURL string) *Client {
	// リトライとHTTP実行の関心は NewRetryHTTPRequester に移譲
	requester := NewRetryHTTPRequester(httpClientTimeout)

	return &Client{
		requester: requester,
		apiURL:    apiURL,
	}
}

// runAudioQuery は /audio_query API を呼び出し、音声合成のためのクエリJSONを返します。
func (c *Client) runAudioQuery(text string, styleID int, ctx context.Context) ([]byte, error) {
	// VOICEVOX特有の関心: URLとクエリパラメータの組み立て
	queryURL := fmt.Sprintf("%s/audio_query", c.apiURL)
	params := url.Values{}
	params.Add("text", text)
	params.Add("speaker", strconv.Itoa(styleID))

	fullURL := queryURL + "?" + params.Encode()

	// Requesterに実行を委譲。リトライ、HTTP接続の関心はRequester側。
	queryBody, err := c.requester.DoRequest(ctx, http.MethodPost, fullURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("オーディオクエリ実行失敗: %w", err)
	}

	return queryBody, nil
}

// runSynthesis は /synthesis API を呼び出し、WAV形式の音声データを返します。
func (c *Client) runSynthesis(queryBody []byte, styleID int, ctx context.Context) ([]byte, error) {
	// VOICEVOX特有の関心: URLとクエリパラメータの組み立て
	synthURL := fmt.Sprintf("%s/synthesis", c.apiURL)
	synthParams := url.Values{}
	synthParams.Add("speaker", strconv.Itoa(styleID))

	fullURL := synthURL + "?" + synthParams.Encode()

	headers := map[string]string{
		"Content-Type": "application/json",
	}

	// Requesterに実行を委譲。リトライ、HTTP接続の関心はRequester側。
	wavData, err := c.requester.DoRequest(ctx, http.MethodPost, fullURL, bytes.NewReader(queryBody), headers)
	if err != nil {
		return nil, fmt.Errorf("音声合成実行失敗: %w", err)
	}

	// VOICEVOX特有の関心: WAVデータ整合性チェック
	if len(wavData) < WavTotalHeaderSize {
		return nil, fmt.Errorf("音声合成APIから無効な（短すぎる）WAVデータが返されました。サイズ: %d", len(wavData))
	}

	return wavData, nil
}

// Get は汎用のGETリクエストを実行します。（Clientの公開メソッドとしてRequesterに委譲）
func (c *Client) Get(url string, ctx context.Context) (*http.Response, error) {
	// リトライなしのGETはRequesterのGetメソッドに委譲
	return c.requester.Get(url, ctx)
}
