package voicevox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// APIClient はVOICEVOXエンジンへのAPIリクエストを処理するクライアントです。
// エンタープライズレベルの堅牢性のため、クライアント設定を含みます。
type APIClient struct {
	client *http.Client
	apiURL string
}

// NewAPIClient は新しいAPIClientインスタンスを初期化します。
// エンタープライズ環境を想定し、接続レベルのタイムアウトを設定します。
func NewAPIClient(apiURL string) *APIClient {
	return &APIClient{
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
		apiURL: apiURL,
	}
}

// runAudioQuery は /audio_query API を呼び出し、音声合成のためのクエリJSONを返します。
func (c *APIClient) runAudioQuery(text string, styleID int, ctx context.Context) ([]byte, error) {
	queryURL := fmt.Sprintf("%s/audio_query", c.apiURL)
	params := url.Values{}
	params.Add("text", text)
	params.Add("speaker", strconv.Itoa(styleID))

	// contextをリクエストに組み込み、タイムアウトやキャンセルを適用
	req, err := http.NewRequestWithContext(ctx, "POST", queryURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("オーディオクエリPOSTリクエスト作成失敗: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		// contextのキャンセルによるエラーを含む
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

// runSynthesis は /synthesis API を呼び出し、WAV形式の音声データを返します。
func (c *APIClient) runSynthesis(queryBody []byte, styleID int, ctx context.Context) ([]byte, error) {
	synthURL := fmt.Sprintf("%s/synthesis", c.apiURL)
	synthParams := url.Values{}
	synthParams.Add("speaker", strconv.Itoa(styleID))

	// contextをリクエストに組み込み
	req, err := http.NewRequestWithContext(ctx, "POST", synthURL+"?"+synthParams.Encode(), bytes.NewReader(queryBody))
	if err != nil {
		return nil, fmt.Errorf("音声合成POSTリクエスト作成失敗: %w", err)
	}
	req.Header.Set("Content-Type", "application/json") // JSONボディを送信することを明示

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("音声合成API呼び出し失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("音声合成APIがエラーを返しました: Status %d, Body: %s", resp.StatusCode, string(errorBody))
	}

	wavData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("音声合成結果の読み込み失敗: %w", err)
	}

	// WAVEデータが空でないことを確認
	if len(wavData) < WavTotalHeaderSize { // WavTotalHeaderSize は wav_utils.go で定義される定数を想定
		return nil, fmt.Errorf("音声合成APIから無効な（短すぎる）WAVデータが返されました")
	}

	return wavData, nil
}
