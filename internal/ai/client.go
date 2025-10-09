package ai

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"text/template"

	"prototypus-ai-doc-go/internal/prompt" // 組み込みプロンプトを取得

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// Client はGemini APIとの通信を管理します。
type Client struct {
	client    *genai.Client
	modelName string
}

// NewClient はGeminiClientを初期化します。ctxを引数に追加
func NewClient(ctx context.Context, modelName string) (*Client, error) {
	// 1. APIキーの取得
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	// 2. クライアントの作成
	// ctx を genai.NewClient に渡す
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	return &Client{
		client:    client,
		modelName: modelName,
	}, nil
}

// Close はクライアントを閉じ、リソースを解放します。
func (c *Client) Close() {
	if c.client != nil {
		c.client.Close() // 以前の動作確認済みコードの通り、このメソッドは存在します
	}
}

// GenerateScript はナレーションスクリプトを生成します。
func (c *Client) GenerateScript(ctx context.Context, inputContent []byte, mode string) (string, error) {

	// 1. プロンプトのテンプレートを取得
	promptTemplateString, err := prompt.GetPromptByMode(mode)
	if err != nil {
		return "", err
	}

	// 2. プロンプトにユーザーの入力テキストを埋め込む
	// 入力はコード差分ではなく、InputTextとして埋め込みます

	// テンプレートの定義（ナレーションスクリプト生成用）
	type InputData struct{ InputText string }

	tmpl, err := template.New("narration_prompt").Parse(promptTemplateString)
	if err != nil {
		return "", fmt.Errorf("プロンプトテンプレートの解析エラー: %w", err)
	}

	data := InputData{InputText: string(inputContent)}
	var fullPrompt bytes.Buffer
	if err := tmpl.Execute(&fullPrompt, data); err != nil {
		return "", fmt.Errorf("プロンプトへの入力埋め込みエラー: %w", err)
	}

	finalPrompt := fullPrompt.String()

	// 3. API呼び出し (提供コードのロジックを流用)
	model := c.client.GenerativeModel(c.modelName)
	resp, err := model.GenerateContent(ctx, genai.Text(finalPrompt))
	if err != nil {
		return "", fmt.Errorf("GenerateContent failed with model %s: %w", c.modelName, err)
	}

	// 4. レスポンスの処理 (提供コードのロジックを流用)
	if resp == nil || len(resp.Candidates) == 0 {
		return "", fmt.Errorf("received empty or invalid response from Gemini API")
	}

	candidate := resp.Candidates[0]

	if candidate.FinishReason != genai.FinishReasonUnspecified && candidate.FinishReason != genai.FinishReasonStop {
		// FinishReasonStop (正常終了) 以外の理由で停止した場合
		return "", fmt.Errorf("API response was blocked or finished prematurely. Reason: %s", candidate.FinishReason.String())
	}

	// その後、コンテンツの有無をチェック
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini response candidate is empty or lacks content parts")
	}

	reviewText, ok := candidate.Content.Parts[0].(genai.Text)
	if !ok {
		return "", fmt.Errorf("API returned non-text part in response")
	}

	return string(reviewText), nil
}
