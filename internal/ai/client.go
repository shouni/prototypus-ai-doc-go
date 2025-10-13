package ai

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"prototypus-ai-doc-go/internal/prompt"
	"text/template"

	"google.golang.org/genai"
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

// buildPrompt は指定されたモードと入力内容に基づいて、APIに渡すプロンプト文字列を構築します。
func buildPrompt(inputContent []byte, mode string) (string, error) {
	// 1. プロンプトのテンプレートを取得
	promptTemplateString, err := prompt.GetPromptByMode(mode)
	if err != nil {
		return "", err
	}

	// 2. プロンプトにユーザーの入力テキストを埋め込む
	type InputData struct{ InputText string }

	// テンプレートの解析
	tmpl, err := template.New("narration_prompt").Parse(promptTemplateString)
	if err != nil {
		return "", fmt.Errorf("プロンプトテンプレートの解析エラー: %w", err)
	}

	// データの埋め込み
	data := InputData{InputText: string(inputContent)}
	var fullPrompt bytes.Buffer
	if err := tmpl.Execute(&fullPrompt, data); err != nil {
		return "", fmt.Errorf("プロンプトへの入力埋め込みエラー: %w", err)
	}

	return fullPrompt.String(), nil
}

// callGenerateContent はAPIを呼び出し、レスポンスからテキストを抽出します。
func (c *Client) callGenerateContent(ctx context.Context, finalPrompt string) (string, error) {
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
		c.modelName,
		contents,
		nil,
	)

	if err != nil {
		return "", fmt.Errorf("GenerateContent failed with model %s: %w", c.modelName, err)
	}

	// 4. レスポンスの処理
	if resp == nil || len(resp.Candidates) == 0 {
		return "", fmt.Errorf("Gemini APIから空または無効なレスポンスが返されました")
	}

	candidate := resp.Candidates[0]

	// 安全性チェック: レスポンスがブロックされていないか確認
	if candidate.FinishReason != genai.FinishReasonUnspecified && candidate.FinishReason != genai.FinishReasonStop {
		return "", fmt.Errorf("APIレスポンスがブロックされたか、途中で終了しました。理由: %v", candidate.FinishReason)
	}

	// コンテンツの有無をチェック
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini レスポンスのコンテンツが空です")
	}

	firstPart := candidate.Content.Parts[0]

	// Textフィールドの値をチェック
	if firstPart.Text == "" {
		// non-text part (e.g., image) の可能性を考慮
		return "", fmt.Errorf("APIは非テキスト形式の応答を返したか、テキストフィールドが空です")
	}

	return firstPart.Text, nil
}

// GenerateScript はナレーションスクリプトを生成します。
func (c *Client) GenerateScript(ctx context.Context, inputContent []byte, mode string) (string, error) {

	// 1. プロンプトの構築
	finalPrompt, err := buildPrompt(inputContent, mode)
	if err != nil {
		return "", err
	}

	// 2. API呼び出しとレスポンスの処理
	return c.callGenerateContent(ctx, finalPrompt)
}
