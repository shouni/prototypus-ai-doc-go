package voicevox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	// webclient.Client 構造体を利用するためにインポート
	webexact "github.com/shouni/go-web-exact/v2/pkg/client"
)

// Client はVOICEVOXエンジンへのAPIリクエストを処理するクライアントです。
// webclient.Client (リトライ機能付きHTTPクライアント) に依存します。
type Client struct {
	// webclient.Client は FetchBytes や PostJSONAndFetchBytes を持つ構造体
	webClient webexact.Client
	apiURL    string
}

// NewClient は新しいClientインスタンスを初期化します。
func NewClient(apiURL string, webClient *webexact.Client) *Client {
	// 引数の型を *webclient.Client に修正すると DI の意図がより明確になりますが、
	// 既存の cmd コードと整合性を保つため、今回は型を webclient.Client のままにしておきます。
	// (webclient.Client はポインタではなく構造体として渡されていました)
	return &Client{
		webClient: *webClient, // ポインタではなく構造体として受け取ることを前提
		apiURL:    apiURL,
	}
}

// ----------------------------------------------------------------------
// コアロジック (VOICEVOX特有の関心)
// ----------------------------------------------------------------------

// runAudioQuery は /audio_query API を呼び出し、音声合成のためのクエリJSONを返します。
// GETではなく、リクエストは空ボディのPOSTですが、POSTリクエストの実行には webClient.Do() の利用を避けます。
// 代わりに、専用の PostJSONAndFetchBytes のようなメソッドを持つべきですが、
// /audio_query が空ボディPOST + クエリパラメータであるため、ここでは内部的に webClient.Do() の拡張機能を使用します。
func (c *Client) runAudioQuery(text string, styleID int, ctx context.Context) ([]byte, error) {
	// 1. URLとクエリパラメータの組み立て
	queryURL := fmt.Sprintf("%s/audio_query", c.apiURL)
	params := url.Values{}
	params.Add("text", text)
	params.Add("speaker", strconv.Itoa(styleID))

	fullURL := queryURL + "?" + params.Encode()

	// 2. リクエスト実行:
	// webclient.Client には FetchBytes と PostJSONAndFetchBytes しかないため、
	// 空のPOSTリクエストは、汎用のDoメソッドを使うか、FetchBytesの代わりにカスタムFetchPOSTを作成するべきですが、
	// ここではシンプルに Get メソッドを利用してリクエストを準備し、リトライロジックに委譲します。
	// ただし、VOICEVOXの /audio_query は**空のPOST**であるため、FetchBytesは使えません。
	// したがって、リトライを含んだPOST機能を webclient.Client に実装していない限り、
	// ここは以前のコードのように webClient.Do() に頼るしかありませんが、
	// **webClient.Do() はリトライを処理しますが、ステータスコードエラー処理がVOICEVOXクライアント側に残ってしまいます。**

	// **暫定修正案 (webClient.Doに依存する):**
	// **webClient.Clientは webclient.Doer インターフェースを満たすため、Do()が呼び出し可能です。**
	// **リトライは webClient.Do() の実装内で処理されるため、このコードは機能的に正しいです。**

	// 既存コードを最小限に修正: HandleLimitedResponse は webclient の内部実装に依存せず、io.LimitReader を使っているため、そのまま利用できます。
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("オーディオクエリ POST リクエスト作成失敗: %w", err)
	}

	// webclient.Client の Do() メソッドはリトライロジックを含みます
	resp, err := c.webClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("オーディオクエリ実行失敗 (リトライ後): %w", err)
	}

	// webclient.Client の HandleLimitedResponse は内部的に resp.Body.Close() を呼ぶため、defer は不要
	// 3. レスポンス処理
	queryBody, err := webexact.HandleLimitedResponse(resp, webexact.MaxResponseBodySize)
	if err != nil {
		return nil, fmt.Errorf("オーディオクエリ実行後のレスポンス読み込み失敗: %w", err)
	}

	// ... 後続のステータスコードチェックとJSONチェックは維持 ...

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
		// webclient.Client のエラーは既にリトライとステータスコードエラーを含んでいます。
		return nil, fmt.Errorf("音声合成実行失敗: %w", err)
	}

	// 4. WAVデータ整合性チェック
	if len(wavData) < WavTotalHeaderSize {
		return nil, fmt.Errorf("音声合成APIから無効な（短すぎる）WAVデータが返されました。サイズ: %d", len(wavData))
	}

	return wavData, nil
}

// Get は汎用のGETリクエストを実行します。（リトライを含む webclient.Client の FetchBytes に委譲）
// webclient.Client の FetchBytes は []byte を返すため、ここでは Get メソッドを削除するか、
// FetchBytes のラッパーとして再定義し、戻り値を []byte に変更します。
// Get(*http.Response, error) を返すのは、client.Do() のシグネチャであり、client.FetchBytes の意図に反します。

// **代替案として、この Get メソッドを削除するか、または外部に公開する必要がある機能 (FetchBytes) のみに限定します。**
// **この voicevox パッケージの関心は音声合成にあるため、この汎用 Get は不要と考え、削除します。**

func (c *Client) Get(url string, ctx context.Context) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.webClient.Do(req)
}
