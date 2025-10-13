package ai

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"text/template"

	"prototypus-ai-doc-go/internal/prompt"

	"google.golang.org/genai"
)

// Client はGemini APIとの通信を管理します。
type Client struct {
	client    *genai.Client
	modelName string
}

// NewClient はGeminiClientを初期化します。ctxを引数に追加
func NewClient(ctx context.Context, modelName string) (*Client, error) {
	// 1. APIキーの取得ロジックを復元
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	// 2. クライアントの作成
	// *genai.ClientConfig を引数に取る形式に対応 (以前のエラーに基づく修正)
	clientConfig := &genai.ClientConfig{
		APIKey: apiKey,
	}

	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	return &Client{
		client:    client,
		modelName: modelName,
	}, nil
}

// GenerateScript はナレーションスクリプトを生成します。
func (c *Client) GenerateScript(ctx context.Context, inputContent []byte, mode string) (string, error) {

	// 1. プロンプトのテンプレートを取得
	promptTemplateString, err := prompt.GetPromptByMode(mode)
	if err != nil {
		return "", err
	}

	// 2. プロンプトにユーザーの入力テキストを埋め込む
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

	// 3. API呼び出し（古いAPI引数形式に合わせる）
	// want (context.Context, string, []*genai.Content, *genai.GenerateContentConfig)

	// 入力コンテンツを作成
	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{Text: finalPrompt},
			},
		},
	}

	// API呼び出しを実行
	resp, err := c.client.Models.GenerateContent(
		ctx,
		c.modelName, // 1st argument: モデル名 (string)
		contents,    // 2nd argument: コンテンツスライス ([]*genai.Content)
		nil,         // 3rd argument: コンフィグ (*genai.GenerateContentConfig)。今回はnilで省略
	)

	if err != nil {
		return "", fmt.Errorf("GenerateContent failed with model %s: %w", c.modelName, err)
	}

	// 4. レスポンスの処理 (古いSDKの型に合わせて修正)
	if resp == nil || len(resp.Candidates) == 0 {
		return "", fmt.Errorf("received empty or invalid response from Gemini API")
	}

	candidate := resp.Candidates[0]

	if candidate.FinishReason != genai.FinishReasonUnspecified && candidate.FinishReason != genai.FinishReasonStop {
		// FinishReason.String() が無い問題を回避するため、%v を使用して文字列化します
		return "", fmt.Errorf("API response was blocked or finished prematurely. Reason: %v", candidate.FinishReason)
	}

	// その後、コンテンツの有無をチェック
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini response candidate is empty or lacks content parts")
	}

	// candidate.Content.Parts[0] の型は *genai.Part (ポインター) です。
	// そのため、Textフィールドに直接アクセスします。
	firstPart := candidate.Content.Parts[0]

	// Textフィールドが存在するかどうかを確認（構造体のTextフィールドを直接参照）
	if firstPart.Text == "" {
		// Textフィールドが無い、または空の場合
		return "", fmt.Errorf("API returned non-text part in response or text field is empty")
	}

	return firstPart.Text, nil
}
