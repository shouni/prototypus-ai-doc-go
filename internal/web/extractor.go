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
const (
	DefaultHTTPTimeout = 30 * time.Second
	MinParagraphLength = 20 // 短すぎる段落を除去するための最小文字数
	MinHeadingLength   = 3  // 短すぎる見出しを除去するための最小文字数

	// 一般的なメインコンテンツのCSSセレクタ
	mainContentSelectors = "article, main, div[role='main'], #main, #content, .post-content, .article-body, .entry-content"
	// 本文から除外したいノイズ要素のCSSセレクタ
	noiseSelectors = ".related-posts, .social-share, .comments, .ad-banner, .advertisement"
	// 抽出対象とするテキスト要素のタグ
	textExtractionTags = "p, h1, h2, h3, h4, h5, h6, li, blockquote, pre"
)

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
	// 一般的なブラウザのUser-Agentを設定
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36")

	// グローバルクライアントを使用
	resp, err := httpClient.Do(req)
	if err != nil {
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
	mainContent := doc.Find(mainContentSelectors).First()

	// メインコンテンツが見つからなかった場合のフォールバック
	if mainContent.Length() == 0 {
		mainContent = doc.Selection.
			Not("header, footer, nav, aside, .sidebar, script, style, form")
	}

	// 3. 本文領域内のノイズをさらに除去
	mainContent.Find(noiseSelectors).Remove()

	// 4. 記事本体内のテキスト要素を取得し、テキストを結合
	mainContent.Find(textExtractionTags).Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())

		// 見出しタグかどうかを判定
		isHeading := s.Is("h1, h2, h3, h4, h5, h6") // textExtractionTags に含まれるすべてに対応

		if isHeading {
			if len(text) > MinHeadingLength {
				// 見出しには '##' をつけて構造を明確にする
				parts = append(parts, "## "+text)
			}
		} else { // 段落、リスト項目など
			if len(text) > MinParagraphLength {
				parts = append(parts, text)
			}
		}
	})

	if len(parts) <= 1 { // タイトルしかない、または何もない場合
		// タイトルのみで本文がない場合、本文抽出フラグは false
		if len(parts) == 1 && strings.HasPrefix(parts[0], "【記事タイトル】") {
			return strings.Join(parts, "\n\n"), false, nil
		}
		return "", false, fmt.Errorf("webページからタイトルまたは記事本文を抽出できませんでした")
	}

	// 記事本文が抽出できた場合
	return strings.Join(parts, "\n\n"), true, nil
}
