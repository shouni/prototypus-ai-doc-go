package web

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time" // timeパッケージを追加

	"github.com/PuerkitoBio/goquery"
)

// FetchAndExtractText はURLからコンテンツを取得し、記事本文を抽出します。
func FetchAndExtractText(url string, ctx context.Context) (string, error) {
	// HTTPリクエストのセットアップ
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("リクエスト作成エラー: %w", err)
	}

	// タイムアウト設定を追加したHTTPクライアント
	client := &http.Client{
		Timeout: 60 * time.Second, // 30秒のタイムアウトを設定
	}

	resp, err := client.Do(req)
	if err != nil {
		// context.Canceled や context.DeadlineExceeded もここで捕捉される
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

	// 2. 記事本文の抽出
	mainContent := doc.Find("article, main, div[role='main']").First()
	if mainContent.Length() == 0 {
		// メインコンテナが見つからなかった場合、ボディ全体を対象にする
		mainContent = doc.Selection
	}

	// 記事本体内の段落や見出しを取得し、テキストを結合
	mainContent.Find("p, h1, h2, h3").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())

		// ノイズ（非常に短いテキスト）を除去。閾値はコンテンツの性質によって調整の余地あり。
		// 見出しタグは重要度が高いため、短いテキストでも許容するロジックを追加しても良い。
		if len(text) > 10 {
			// 段落区切りとして二重改行を挿入
			parts = append(parts, text)
		}
	})

	if len(parts) == 0 {
		return "", fmt.Errorf("記事本文とタイトルを抽出できませんでした。")
	}
	if len(parts) == 1 && strings.HasPrefix(parts[0], "【記事タイトル】") {
		// タイトルのみで本文がない場合、警告として扱い、タイトルはAIに渡す（ユーザーの意図を尊重）
		// 必要に応じて、ログ出力などでユーザーに警告を伝えることが望ましい。
		fmt.Fprintf(os.Stderr, "警告: 記事本文が見つかりませんでした。タイトルのみで処理を続行します。\n")
	}

	// 抽出されたテキストを結合して返す
	return strings.Join(parts, "\n\n"), nil
}
