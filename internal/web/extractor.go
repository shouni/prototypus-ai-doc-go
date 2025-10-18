package web

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/PuerkitoBio/goquery"
	webextractor "github.com/shouni/go-web-exact/pkg/web"
)

// ----------------------------------------------------------------------
// 外部パッケージの依存性の実装 (go-web-exact/pkg/web の Fetcher インターフェースを満たす)
// ----------------------------------------------------------------------

const (
	DefaultHTTPTimeout = 30 * time.Second
	userAgent          = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36"
)

var httpClient = &http.Client{
	Timeout: DefaultHTTPTimeout,
}

// HTTPAwareFetcher は webextractor.Fetcher インターフェースの実装です。
type HTTPAwareFetcher struct{}

// FetchDocument は goquery.Document を取得する具体的な実装です。
func (*HTTPAwareFetcher) FetchDocument(url string, ctx context.Context) (*goquery.Document, error) {
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

// ----------------------------------------------------------------------
// メインのラッパー関数
// ----------------------------------------------------------------------

// FetchAndExtractText は、CLIツールからの呼び出しを受け付け、
// webextractor パッケージの機能を利用してコンテンツ抽出を実行します。
func FetchAndExtractText(url string, ctx context.Context) (text string, hasBodyFound bool, err error) {
	// 1. Fetcher の実装をインスタンス化
	fetcher := &HTTPAwareFetcher{}

	// 2. Extractor のインスタンス化 (DI)
	extractor := webextractor.NewExtractor(fetcher)

	// 3. 外部パッケージのメソッドを呼び出す
	return extractor.FetchAndExtractText(url, ctx)
}
