package poster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// PostPayload はAPIに送信するデータの構造体です。
type PostPayload struct {
	Title     string `json:"title"`
	Mode      string `json:"mode"`
	Timestamp string `json:"timestamp"`
	Content   string `json:"content"`
}

// PostToAPI は生成されたスクリプトを指定されたAPIエンドポイントに投稿します。
// APIエンドポイントは環境変数 POST_API_URL から取得します。
func PostToAPI(title string, mode string, scriptContent string) error {
	apiURL := os.Getenv("POST_API_URL")
	if apiURL == "" {
		// URLが設定されていない場合はエラーとせず、単に処理をスキップ
		return nil
	}

	// 投稿するデータを作成
	payload := PostPayload{
		Title:     title,
		Mode:      mode,
		Timestamp: time.Now().Format(time.RFC3339),
		Content:   scriptContent,
	}
	payloadBytes, _ := json.Marshal(payload)

	// APIにPOSTリクエストを送信
	resp, err := http.Post(apiURL, "application/json", bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("APIへのリクエスト失敗: %w", err)
	}
	defer resp.Body.Close()

	// 200番台以外のステータスコードはエラーとして処理
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("APIがエラーを返しました: %s (Status: %d)", resp.Status, resp.StatusCode)
	}

	return nil
}
