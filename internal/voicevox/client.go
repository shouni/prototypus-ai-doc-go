package voicevox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	webexact "github.com/shouni/go-web-exact/v2/pkg/client"
)

// Client はVOICEVOXエンジンへのAPIリクエストを処理するクライアントです。
// webclient.Client (リトライ機能付きHTTPクライアント) に依存します。
type Client struct {
	webClient *webexact.Client
	apiURL    string
}

// NewClient は新しいClientインスタンスを初期化します。
func NewClient(apiURL string, webClient *webexact.Client) *Client {
	return &Client{
		webClient: webClient,
		apiURL:    apiURL,
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

	// 2. リクエスト実行:
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("オーディオクエリ POST リクエスト作成失敗: %w", err)
	}

	// webclient.Client の Do() メソッドはリトライロジックを含みます
	resp, err := c.webClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("オーディオクエリ実行失敗 (リトライ後): %w", err)
	}

	// 3. レスポンス処理
	queryBody, err := webexact.HandleLimitedResponse(resp, webexact.MaxResponseBodySize)
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

	// VOICEVOXクエリに必須のキー 'accent_phrases' の存在をチェック
	if _, ok := jsonCheck["accent_phrases"]; !ok {
		return nil, fmt.Errorf("オーディオクエリが必須フィールド 'accent_phrases' を含みません。VOICEVOXエンジンがテキストを処理できなかった可能性があります。Body: %s", string(queryBody))
	}

	return queryBody, nil
}

// runSynthesis は /synthesis API を呼び出し、WAV形式の音声データを返します。
// これは JSONボディをPOSTするリクエストであるため、webclient.PostJSONAndFetchBytes を利用できます。
func (c *Client) runSynthesis(queryBody []byte, styleID int, ctx context.Context) ([]byte, error) {
	// 1. URLとクエリパラメータの組み立て
	synthURL := fmt.Sprintf("%s/synthesis", c.apiURL)
	synthParams := url.Values{}
	synthParams.Add("speaker", strconv.Itoa(styleID))

	fullURL := synthURL + "?" + synthParams.Encode()
	wavData, err := c.webClient.PostJSONAndFetchBytes(fullURL, json.RawMessage(queryBody), ctx)
	if err != nil {
		return nil, fmt.Errorf("音声合成実行失敗: %w", err)
	}

	// 4. WAVデータ整合性チェック
	if len(wavData) < WavTotalHeaderSize {
		return nil, fmt.Errorf("音声合成APIから無効な（短すぎる）WAVデータが返されました。サイズ: %d", len(wavData))
	}

	return wavData, nil
}

// Get は汎用のGETリクエストを実行します。
func (c *Client) Get(url string, ctx context.Context) ([]byte, error) {
	// webClient.FetchBytes は内部でリクエスト作成、実行、レスポンス処理、ボディクローズを行います。
	data, err := c.webClient.FetchBytes(url, ctx)
	if err != nil {
		return nil, fmt.Errorf("GETリクエスト実行失敗: %w", err)
	}
	return data, nil
}
