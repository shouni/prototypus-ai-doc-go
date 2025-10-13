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

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("JSONマーシャリングに失敗しました: %w", err)
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("APIリクエストの作成に失敗しました: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// client.Do() を使用してリクエストを送信
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("APIへのリクエスト失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("APIがエラーを返しました: %s (Status: %d)", resp.Status, resp.StatusCode)
	}

	return nil
}
