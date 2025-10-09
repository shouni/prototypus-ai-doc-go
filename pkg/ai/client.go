package ai

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"text/template"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
	"prototypus-ai-doc-go/pkg/prompt" // 組み込みプロンプトを取得
)

// Client はGemini APIとの通信を管理します。
type Client struct {
	client    *genai.Client
	modelName string
}

// NewClient はGeminiClientを初期化します。
func NewClient(modelName string) (*Client, error) {
	// 1. APIキーの取得
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	// 2. クライアントの作成
	// NewClientはContextを受け取らない形式に戻します。
	client, err := genai.NewClient(context.Background(), option.WithAPIKey(apiKey))
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

	// ... (ブロック理由のチェックなど、提供コードのエラーハンドリングを続行)

	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		if candidate.FinishReason != genai.FinishReasonUnspecified {
			return "", fmt.Errorf("API response was blocked or finished prematurely. Reason: %s", candidate.FinishReason.String())
		}
		return "", fmt.Errorf("Gemini response candidate is empty or lacks content parts")
	}

	reviewText, ok := candidate.Content.Parts[0].(genai.Text)
	if !ok {
		return "", fmt.Errorf("API returned non-text part in response")
	}

	return string(reviewText), nil
}
