package voicevox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/shouni/go-web-exact/pkg/httpclient"
)

// Client はVOICEVOXエンジンへのAPIリクエストを処理するクライアントです。
// httpclient.HTTPClient (または互換インターフェース) に依存します。
type Client struct {
	httpClient httpclient.HTTPClient // httpclient.HTTPClient インターフェースに依存
	apiURL     string
}

// NewClient は新しいClientインスタンスを初期化します。
func NewClient(apiURL string, client httpclient.HTTPClient) *Client {
	return &Client{
		httpClient: client,
		apiURL:     apiURL,
	}
}

// ----------------------------------------------------------------------
// コアロジック (VOICEVOX特有の関心)
// ----------------------------------------------------------------------

// runAudioQuery は /audio_query API を呼び出し、音声合成のためのクエリJSONを返します。
func (c *Client) runAudioQuery(text string, styleID int, ctx context.Context) ([]byte, error) {
	// 1. URLとクエリパラメータの組み立て
	queryURL := fmt.Sprintf("%s/audio_query", c.apiURL)
	params := url.Values{}
	params.Add("text", text)
	params.Add("speaker", strconv.Itoa(styleID))

	fullURL := queryURL + "?" + params.Encode()

	// 2. 実行: httpclient.HTTPClient の Do を利用
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("オーディオクエリ POST リクエスト作成失敗: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// ネットワークエラー、リトライ後の最終失敗など
		return nil, fmt.Errorf("オーディオクエリ実行失敗 (リトライ後): %w", err)
	}

	// 3. レスポンス処理
	queryBody, err := httpclient.HandleLimitedResponse(resp, httpclient.MaxResponseBodySize)
	if err != nil {
		return nil, fmt.Errorf("オーディオクエリ実行後のレスポンス読み込み失敗: %w", err)
	}

	// 4. 最終的なステータスコードチェック
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("オーディオクエリ実行失敗 (ステータスコード %d): %s", resp.StatusCode, string(queryBody))
	}

	// 5. 必須チェック: ボディが有効な VOICEVOX クエリ JSON であることを確認
	if len(queryBody) == 0 {
		return nil, fmt.Errorf("オーディオクエリ実行成功 (2xx) ですが、ボディが空です。")
	}

	// JSONの有効性をチェック
	var jsonCheck map[string]interface{}
	if json.Unmarshal(queryBody, &jsonCheck) != nil {
		return nil, fmt.Errorf("オーディオクエリ実行成功 (2xx) ですが、不正な JSON が返されました: %s", string(queryBody))
	}

	// 💡 最終修正のコア: VOICEVOXクエリに必須のキー 'accent_phrases' の存在をチェック
	if _, ok := jsonCheck["accent_phrases"]; !ok {
		// このエラーは、不正なクエリボディによる 422 エラーが /synthesis で発生する代わりに、ここで捕捉されます。
		return nil, fmt.Errorf("オーディオクエリが必須フィールド 'accent_phrases' を含みません。VOICEVOXエンジンがテキストを処理できなかった可能性があります。Body: %s", string(queryBody))
	}

	return queryBody, nil
}

// runSynthesis は /synthesis API を呼び出し、WAV形式の音声データを返します。
func (c *Client) runSynthesis(queryBody []byte, styleID int, ctx context.Context) ([]byte, error) {
	// 1. URLとクエリパラメータの組み立て
	synthURL := fmt.Sprintf("%s/synthesis", c.apiURL)
	synthParams := url.Values{}
	synthParams.Add("speaker", strconv.Itoa(styleID))

	fullURL := synthURL + "?" + synthParams.Encode()

	// 2. 実行: POSTリクエストの準備
	// queryBody を直接リクエストボディとして使用
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(queryBody))
	if err != nil {
		return nil, fmt.Errorf("音声合成 POST リクエスト作成失敗: %w", err)
	}
	// JSONデータを送信するため Content-Type を明示的に設定
	req.Header.Set("Content-Type", "application/json")

	// 3. リクエスト実行 (リトライは c.httpClient.Do() 側で処理される)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// ネットワークエラー、リトライ後の最終失敗など
		return nil, fmt.Errorf("音声合成実行失敗 (リトライ後): %w", err)
	}

	// 4. レスポンス処理
	wavData, err := httpclient.HandleLimitedResponse(resp, httpclient.MaxResponseBodySize)
	if err != nil {
		return nil, fmt.Errorf("音声合成実行後のレスポンス読み込み失敗: %w", err)
	}

	// 5. ステータスコードチェック
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 422 エラーはこのブロックで捕捉され、エラーボディが返されます。
		return nil, fmt.Errorf("音声合成実行失敗: ステータスコード %d, ボディ: %s", resp.StatusCode, string(wavData))
	}

	// 6. WAVデータ整合性チェック
	if len(wavData) < WavTotalHeaderSize {
		return nil, fmt.Errorf("音声合成APIから無効な（短すぎる）WAVデータが返されました。サイズ: %d", len(wavData))
	}

	return wavData, nil
}

// Get は汎用のGETリクエストを実行します。（リトライを含む httpclient.Client の Do に委譲）
func (c *Client) Get(url string, ctx context.Context) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	// リトライと共通ヘッダー処理は c.httpClient.Do(req) の実装に委譲
	return c.httpClient.Do(req)
}
