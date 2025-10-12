package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// HTTPリクエストおよびノイズ除去の定数
const DefaultHTTPTimeout = 30 * time.Second
const MinParagraphLength = 20 // 短すぎる段落を除去するための最小文字数
const MinHeadingLength = 3    // 短すぎる見出しを除去するための最小文字数 (見出しは短くても重要)

// httpClient はコネクションプールを再利用するためにパッケージ内で共有されるHTTPクライアントです。
var httpClient = &http.Client{
	Timeout: DefaultHTTPTimeout,
}

// FetchAndExtractText はURLからコンテンツを取得し、記事本文を抽出します。
// hasBodyFound は記事本文が抽出できたかどうかを示すフラグです。
func FetchAndExtractText(url string, ctx context.Context) (text string, hasBodyFound bool, err error) {
	// HTTPリクエストのセットアップ
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", false, fmt.Errorf("リクエスト作成エラー: %w", err)
	}

	// グローバルクライアントを使用
	resp, err := httpClient.Do(req)
	if err != nil {
		// context.Canceled や context.DeadlineExceeded もここで捕捉される
		return "", false, fmt.Errorf("HTTPリクエストエラー: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("HTTPステータスコードエラー: %d", resp.StatusCode)
	}

	// goqueryによるHTMLの解析
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("HTML解析エラー: %w", err)
	}

	var parts []string

	// 1. ページタイトル (AI入力として重要)
	pageTitle := doc.Find("title").First().Text()
	if strings.TrimSpace(pageTitle) != "" {
		parts = append(parts, "【記事タイトル】 "+pageTitle)
	}

	// 2. 記事本文の抽出
	mainContent := doc.Find("article, main, div[role='main']").First()

	// メインコンテンツが見つからなかった場合のフォールバックとノイズ除去の強化
	if mainContent.Length() == 0 {
		// より具体的な代替セレクタを試す (例: 多くのブログで使われるクラス名)
		// もしこれらのセレクタが見つかれば、より狭い範囲で抽出できる
		alternativeContent := doc.Find(".post-content, .article-body, #content").First()
		if alternativeContent.Length() > 0 {
			mainContent = alternativeContent
		} else {
			// それでも見つからない場合に、一般的なノイズ要素を除外したドキュメント全体を対象とする
			mainContent = doc.Selection.
				Not("header, footer, nav, aside, .sidebar, .ad-banner, .advertisement, script, style, form") // script, style, formも追加
		}
	}

	// 記事本体内の段落や見出しを取得し、テキストを結合
	mainContent.Find("p, h1, h2, h3").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())

		isParagraph := s.Is("p")

		if isParagraph { // 段落タグの場合
			if len(text) > MinParagraphLength {
				// 段落区切りとして二重改行を挿入
				parts = append(parts, text)
			}
		} else { // 見出しタグ (h1, h2, h3) の場合
			if len(text) > MinHeadingLength {
				parts = append(parts, text)
			}
		}
	})

	if len(parts) == 0 {
		// タイトルも記事本文も抽出できなかった場合
		return "", false, fmt.Errorf("Webページからタイトルも記事本文も抽出できませんでした。セレクタの調整が必要かもしれません。")
	}

	if len(parts) == 1 && strings.HasPrefix(parts[0], "【記事タイトル】") {
		// タイトルのみで本文がない場合、タイトルは返すが本文抽出フラグは false
		return strings.Join(parts, "\n\n"), false, nil
	}

	// 記事本文が抽出できた場合
	return strings.Join(parts, "\n\n"), true, nil
}
