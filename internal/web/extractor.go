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
	userAgent            = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36"
	titlePrefix          = "【記事タイトル】 "
)

var httpClient = &http.Client{
	Timeout: DefaultHTTPTimeout,
}

// FetchAndExtractText は指定されたURLからコンテンツを取得し、整形されたテキストを抽出します。
func FetchAndExtractText(url string, ctx context.Context) (text string, hasBodyFound bool, err error) {
	doc, err := fetchHTML(url, ctx)
	if err != nil {
		return "", false, err
	}

	return extractContentText(doc)
}

// fetchHTML はURLからHTMLを取得し、goquery.Documentを返します。
func fetchHTML(url string, ctx context.Context) (*goquery.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("リクエスト作成に失敗しました: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTPリクエストに失敗しました: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTPステータスコードエラー: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("HTML解析に失敗しました: %w", err)
	}

	return doc, nil
}

// extractContentText はgoquery.Documentから本文とタイトルを抽出し、整形します。
func extractContentText(doc *goquery.Document) (text string, hasBodyFound bool, err error) {
	var parts []string

	// 1. ページタイトルを抽出
	pageTitle := strings.TrimSpace(doc.Find("title").First().Text())
	if pageTitle != "" {
		parts = append(parts, titlePrefix+pageTitle)
	}

	// 2. メインコンテンツの特定
	mainContent := findMainContent(doc)

	// 3. ノイズ要素の除去
	mainContent.Find(noiseSelectors).Remove()

	// 4. 記事本体内のテキスト要素を取得し、テキストを結合
	mainContent.Find(textExtractionTags).Each(func(i int, s *goquery.Selection) {
		if content := processElement(s); content != "" {
			parts = append(parts, content)
		}
	})

	// 5. 抽出結果の検証
	return validateAndFormatResult(parts)
}

// findMainContent はメインコンテンツと見なされるgoquery.Selectionを返します。
func findMainContent(doc *goquery.Document) *goquery.Selection {
	mainContent := doc.Find(mainContentSelectors).First()
	if mainContent.Length() == 0 {
		// メインコンテンツのセレクタで見つからなかった場合、
		// 一般的な装飾要素を除外したドキュメント全体を対象とする
		mainContent = doc.Selection.
			Not("header, footer, nav, aside, .sidebar, script, style, form")
	}
	return mainContent
}

// processElement は個々のHTML要素からテキストを抽出し、整形します。
func processElement(s *goquery.Selection) string {
	if s.Is("table") {
		return processTable(s)
	}

	text := strings.TrimSpace(s.Text())
	isHeading := s.Is("h1, h2, h3, h4, h5, h6")

	if text == "" {
		return ""
	}

	if isHeading {
		if len(text) > MinHeadingLength {
			// 見出しはMarkdown風に整形
			return "## " + text
		}
	} else {
		if len(text) > MinParagraphLength {
			// 段落はそのまま
			return text
		}
	}
	return ""
}

// processTable はテーブル要素からテキストを抽出し、整形します。
func processTable(s *goquery.Selection) string {
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
	// テーブル全体を1つのテキストブロックとして結合
	if len(tableContent) > 0 {
		return strings.Join(tableContent, "\n")
	}
	return ""
}

// validateAndFormatResult は抽出されたパーツを検証し、最終的な結果を返します。
func validateAndFormatResult(parts []string) (text string, hasBodyFound bool, err error) {
	if len(parts) == 0 {
		return "", false, fmt.Errorf("webページから何も抽出できませんでした")
	}

	// タイトルのみの場合の判定
	isTitleOnly := len(parts) == 1 && strings.HasPrefix(parts[0], titlePrefix)
	if isTitleOnly {
		return parts[0], false, nil
	}

	// 抽出されたパーツを改行で結合
	return strings.Join(parts, "\n\n"), true, nil
}
