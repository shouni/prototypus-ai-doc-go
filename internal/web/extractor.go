package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// FetchAndExtractText はURLからコンテンツを取得し、記事本文を抽出します。
// 汎用的な記事抽出のため、<article>または<main>タグ内のテキストを優先します。
func FetchAndExtractText(url string, ctx context.Context) (string, error) {
	// HTTPリクエストのセットアップ
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("リクエスト作成エラー: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTPリクエストエラー: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTPステータスコードエラー: %d", resp.StatusCode)
	}

	// goqueryによるHTMLの解析
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("HTML解析エラー: %w", err)
	}

	var parts []string

	// 1. ページタイトル (AI入力として重要)
	pageTitle := doc.Find("title").First().Text()
	if strings.TrimSpace(pageTitle) != "" {
		parts = append(parts, "【記事タイトル】 "+pageTitle)
	}

	// 2. 記事本文の抽出 (メインコンテンツエリアの特定)
	// 多くのブログや記事サイトは <article> または <main> タグ内に本文を格納します。
	// より広い範囲をカバーするため、一般的なセレクタを組み合わせます。

	// メインコンテナを特定: article, main, または role="main" を持つ div
	mainContent := doc.Find("article, main, div[role='main']").First()
	if mainContent.Length() == 0 {
		// メインコンテナが見つからなかった場合、ボディ全体を対象にする
		mainContent = doc.Selection
	}

	// 記事本体内の段落や見出しを取得し、テキストを結合
	mainContent.Find("p, h1, h2, h3").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		// ノイズ（非常に短いテキスト）を除去
		if len(text) > 10 {
			// 段落区切りとして二重改行を挿入
			parts = append(parts, text)
		}
	})

	if len(parts) <= 1 {
		// タイトルのみで本文がない場合、エラーではなく警告として処理（AIにタイトルだけでも渡す）
		if len(parts) == 0 {
			return "", fmt.Errorf("記事本文とタイトルを抽出できませんでした。")
		}
	}

	// 抽出されたテキストを結合して返す
	return strings.Join(parts, "\n\n"), nil
}
