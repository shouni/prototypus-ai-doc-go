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

// Client はVOICEVOXエンジンへのAPIリクエストを処理するクライアントです。
// APIClientからClientに名称を変更しました。
type Client struct {
	client      *http.Client
	apiURL      string
	retryConfig retry.Config // リトライロジック流用
}

const (
	// HTTPクライアントのタイムアウトを一元管理
	httpClientTimeout = 120 * time.Second

	// サイトからのブロックを避けるためのUser-Agent (httpclientから流用)
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36"

	// MaxResponseBodySize は、あらゆるHTTPレスポンスボディの最大読み込みサイズ (httpclientから流用)
	MaxResponseBodySize = int64(10 * 1024 * 1024) // 10MB
)

// NonRetryableHTTPError はHTTP 4xx系のステータスコードエラーを示すカスタムエラー型です。(httpclientから流用)
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

// IsNonRetryableError は与えられたエラーが非リトライ対象のHTTPエラーであるかを判断します。(httpclientから流用)
func IsNonRetryableError(err error) bool {
	if err == nil {
		return false
	}
	var nonRetryable *NonRetryableHTTPError
	return errors.As(err, &nonRetryable)
}

// NewClient は新しいClientインスタンスを初期化します。(NewAPIClientからNewClientに名称変更)
func NewClient(apiURL string) *Client {
	return &Client{
		client: &http.Client{
			Timeout: httpClientTimeout, // 120秒に設定
		},
		apiURL:      apiURL,
		retryConfig: retry.DefaultConfig(), // デフォルトのリトライ設定を適用
	}
}

// addCommonHeaders は共通のHTTPヘッダーを設定します。
func (c *Client) addCommonHeaders(req *http.Request) {
	req.Header.Set("User-Agent", UserAgent)
}

// isHTTPRetryableError はエラーがHTTPリトライ対象かどうかを判定します。
func (c *Client) isHTTPRetryableError(err error) bool {
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
func (c *Client) doWithRetry(ctx context.Context, operationName string, op func() error) error {
	return retry.Do(
		ctx,
		c.retryConfig,
		operationName,
		op,
		c.isHTTPRetryableError,
	)
}

// handleResponseはHTTPレスポンスを処理し、成功した場合はボディをバイト配列として返します。
// 5xxエラーはリトライ可能なエラーとして返し、4xxエラーはNonRetryableHTTPErrorとして返します。
func handleResponse(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()

	if resp.ContentLength > 0 && resp.ContentLength > MaxResponseBodySize {
		return nil, fmt.Errorf("レスポンスボディが最大サイズ (%dバイト) を超えました", MaxResponseBodySize)
	}

	limitedReader := io.LimitReader(resp.Body, MaxResponseBodySize)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("レスポンスボディの読み込みに失敗しました: %w", err)
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

// Get は汎用のGETリクエストを実行します。（リトライなし）
func (c *Client) Get(url string, ctx context.Context) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.addCommonHeaders(req) // User-Agentを追加
	// contextを付与したリクエストを実行
	return c.client.Do(req)
}

// runAudioQuery は /audio_query API を呼び出し、音声合成のためのクエリJSONを返します。
func (c *Client) runAudioQuery(text string, styleID int, ctx context.Context) ([]byte, error) {
	queryURL := fmt.Sprintf("%s/audio_query", c.apiURL)
	params := url.Values{}
	params.Add("text", text)
	params.Add("speaker", strconv.Itoa(styleID))

	var queryBody []byte

	// リトライロジックでAPI呼び出しをラップ
	op := func() error {
		req, err := http.NewRequestWithContext(ctx, "POST", queryURL+"?"+params.Encode(), nil)
		if err != nil {
			return fmt.Errorf("オーディオクエリPOSTリクエスト作成失敗: %w", err)
		}
		c.addCommonHeaders(req) // User-Agentを追加

		resp, err := c.client.Do(req)
		if err != nil {
			// ネットワークエラー、Contextエラーなど
			return fmt.Errorf("オーディオクエリAPI呼び出し失敗: %w", err)
		}

		// httpclientから流用したhandleResponseでエラーとリトライ可否を判定
		queryBody, err = handleResponse(resp)
		return err
	}

	err := c.doWithRetry(
		ctx,
		fmt.Sprintf("オーディオクエリ(%s)の実行", text),
		op,
	)

	if err != nil {
		return nil, err
	}

	return queryBody, nil
}

// runSynthesis は /synthesis API を呼び出し、WAV形式の音声データを返します。
func (c *Client) runSynthesis(queryBody []byte, styleID int, ctx context.Context) ([]byte, error) {
	synthURL := fmt.Sprintf("%s/synthesis", c.apiURL)
	synthParams := url.Values{}
	synthParams.Add("speaker", strconv.Itoa(styleID))

	var wavData []byte

	// リトライロジックでAPI呼び出しをラップ
	op := func() error {
		req, err := http.NewRequestWithContext(ctx, "POST", synthURL+"?"+synthParams.Encode(), bytes.NewReader(queryBody))
		if err != nil {
			return fmt.Errorf("音声合成POSTリクエスト作成失敗: %w", err)
		}
		c.addCommonHeaders(req)
		req.Header.Set("Content-Type", "application/json") // JSONボディを送信することを明示

		resp, err := c.client.Do(req)
		if err != nil {
			// ネットワークエラー、Contextエラーなど
			return fmt.Errorf("音声合成API呼び出し失敗: %w", err)
		}

		// httpclientから流用したhandleResponseでエラーとリトライ可否を判定
		wavData, err = handleResponse(resp)
		if err != nil {
			return err
		}

		// WAVEデータが空でないことを確認 (wav_utils.goで定義された定数を使用)
		if len(wavData) < WavTotalHeaderSize {
			return fmt.Errorf("音声合成APIから無効な（短すぎる）WAVデータが返されました。サイズ: %d", len(wavData))
		}

		return nil
	}

	err := c.doWithRetry(
		ctx,
		fmt.Sprintf("音声合成(Style %d)の実行", styleID),
		op,
	)

	if err != nil {
		return nil, err
	}

	return wavData, nil
}
