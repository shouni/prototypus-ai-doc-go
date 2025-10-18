package poster

import (
	"context"
	"fmt"
	"os"
	"time"

	httpclient "github.com/shouni/go-web-exact/pkg/httpclient"
)

// PostPayload はAPIに送信するデータの構造体です。
type PostPayload struct {
	Title     string `json:"title"`
	Mode      string `json:"mode"`
	Timestamp string `json:"timestamp"`
	Content   string `json:"content"`
}

// PostToAPI は生成されたスクリプトを指定されたAPIエンドポイントに投稿します。
func PostToAPI(title string, mode string, scriptContent string) error {
	apiURL := os.Getenv("POST_API_URL")
	if apiURL == "" {
		return nil
	}

	payload := PostPayload{
		Title:     title,
		Mode:      mode,
		Timestamp: time.Now().Format(time.RFC3339),
		Content:   scriptContent,
	}

	// 独自のクライアントを初期化（タイムアウトはライブラリの既定値を使用）
	// ポスターにはタイムアウトフラグがないため、ここでクライアントのデフォルトを使用
	client := httpclient.New(httpclient.DefaultHTTPTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), httpclient.DefaultHTTPTimeout)
	defer cancel()

	// PostJSONAndFetchBytes を呼び出し
	_, err := client.PostJSONAndFetchBytes(apiURL, payload, ctx)
	if err != nil {
		// PostJSONAndFetchBytesはリトライ後の最終エラーを返します。
		return fmt.Errorf("外部APIへの投稿失敗（リトライ後）: %w", err)
	}

	// PostJSONAndFetchBytes がエラーを返さなかった場合、2xxステータスで成功したと見なされます。
	return nil
}
