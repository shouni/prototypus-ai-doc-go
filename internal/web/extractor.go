package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// (定数定義などは変更なし)
const (
	DefaultHTTPTimeout   = 30 * time.Second
	MinParagraphLength   = 20
	MinHeadingLength     = 3
	mainContentSelectors = "article, main, div[role='main'], #main, #content, .post-content, .article-body, .entry-content"
	noiseSelectors       = ".related-posts, .social-share, .comments, .ad-banner, .advertisement"
	textExtractionTags   = "p, h1, h2, h3, h4, h5, h6, li, blockquote, pre, table"
)

var httpClient = &http.Client{
	Timeout: DefaultHTTPTimeout,
}

func FetchAndExtractText(url string, ctx context.Context) (text string, hasBodyFound bool, err error) {

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", false, fmt.Errorf("リクエスト作成エラー: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("HTTPリクエストエラー: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("HTTPステータスコードエラー: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("HTML解析エラー: %w", err)
	}

	var parts []string
	pageTitle := doc.Find("title").First().Text()
	if strings.TrimSpace(pageTitle) != "" {
		parts = append(parts, "【記事タイトル】 "+pageTitle)
	}

	mainContent := doc.Find(mainContentSelectors).First()
	if mainContent.Length() == 0 {
		mainContent = doc.Selection.
			Not("header, footer, nav, aside, .sidebar, script, style, form")
	}
	mainContent.Find(noiseSelectors).Remove()

	// 4. 記事本体内のテキスト要素を取得し、テキストを結合
	mainContent.Find(textExtractionTags).Each(func(i int, s *goquery.Selection) {

		if s.Is("table") {
			var tableContent []string
			// テーブルの各行(tr)をループ
			s.Find("tr").Each(func(rowIndex int, row *goquery.Selection) {
				var rowTexts []string
				// 行の中の各セル(th, td)をループ
				row.Find("th, td").Each(func(cellIndex int, cell *goquery.Selection) {
					rowTexts = append(rowTexts, strings.TrimSpace(cell.Text()))
				})
				// セルのテキストを "|" で結合して1行の文字列にする
				tableContent = append(tableContent, strings.Join(rowTexts, " | "))
			})
			// テーブル全体を1つのテキストブロックとして追加
			if len(tableContent) > 0 {
				parts = append(parts, strings.Join(tableContent, "\n"))
			}
			return // テーブルの処理はここで終わり、次の要素へ
		}

		text := strings.TrimSpace(s.Text())
		isHeading := s.Is("h1, h2, h3, h4, h5, h6")

		if isHeading {
			if len(text) > MinHeadingLength {
				parts = append(parts, "## "+text)
			}
		} else {
			if len(text) > MinParagraphLength {
				parts = append(parts, text)
			}
		}
	})

	// (以降の処理は変更なし)
	if len(parts) <= 1 {
		if len(parts) == 1 && strings.HasPrefix(parts[0], "【記事タイトル】") {
			return strings.Join(parts, "\n\n"), false, nil
		}
		return "", false, fmt.Errorf("webページからタイトルも記事本文も抽出できませんでした")
	}

	return strings.Join(parts, "\n\n"), true, nil
}
