package voicevox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// ----------------------------------------------------------------------
// 話者タグとVOICEVOXスタイルタグの定義
// ----------------------------------------------------------------------

// SpeakerMapping は、VOICEVOX API名とツールで使用する短縮タグのペアを定義します。
type SpeakerMapping struct {
	APIName string // 例: "四国めたん"
	ToolTag string // 例: "[めたん]"
}

// SupportedSpeakers は、このツールがサポートするすべての話者の一覧です。
// NOTE: このリストのみが話者定義の唯一の情報源となります。
var SupportedSpeakers = []SpeakerMapping{
	{APIName: "四国めたん", ToolTag: "[めたん]"},
	{APIName: "ずんだもん", ToolTag: "[ずんだもん]"},
}

const (
	// VOICEVOXのスタイル名と一致させる定数
	VvTagNormal   = "[ノーマル]"
	VvTagAmaama   = "[あまあま]"
	VvTagTsuntsun = "[ツンツン]"
	VvTagSexy     = "[セクシー]"
	VvTagWhisper  = "[ささやき]"
)

// VOICEVOX APIのスタイル名からツールのタグ定数へのマッピング
var styleApiNameToToolTag = map[string]string{
	"ノーマル": VvTagNormal,
	"あまあま": VvTagAmaama,
	"ツンツン": VvTagTsuntsun,
	"セクシー": VvTagSexy,
	"ささやき": VvTagWhisper,
}

// ----------------------------------------------------------------------
// スタイルIDの動的データ構造とロードロジック
// ----------------------------------------------------------------------

// SpeakerData はVOICEVOXから動的に取得した全話者・スタイル情報を保持する
type SpeakerData struct {
	StyleIDMap       map[string]int    // 例: "[めたん][ノーマル]" -> 2
	DefaultStyleMap  map[string]string // 例: "[めたん]" -> "[めたん][ノーマル]" (フォールバック用)
	apiNameToToolTag map[string]string // 内部で使用するAPI名 -> ToolTagのマップ
}

// VVSpeaker はVOICEVOXの /speakers APIの応答JSON構造の一部に対応する型
type VVSpeaker struct {
	Name   string `json:"name"`
	Styles []struct {
		Name string `json:"name"`
		ID   int    `json:"id"`
	} `json:"styles"`
}

// LoadSpeakers は /speakers エンドポイントからデータを取得し、SpeakerDataを構築します。
// client.Get() は []byte を返し、通信エラーやステータスコードエラーはエラーとして返ると仮定します。
func LoadSpeakers(ctx context.Context, client *Client) (*SpeakerData, error) {
	// 1. 静的なSupportedSpeakersから、内部使用のためのマップを構築
	apiNameToToolTag := make(map[string]string)
	for _, mapping := range SupportedSpeakers {
		apiNameToToolTag[mapping.APIName] = mapping.ToolTag
	}

	speakersURL := fmt.Sprintf("%s/speakers", client.apiURL)

	// 変更点1: client.Get() は []byte を返すため、 resp や defer resp.Body.Close() を削除
	bodyBytes, err := client.Get(speakersURL, ctx)

	// 変更点2: 通信エラーや 4xx/5xx ステータスコードエラーは client.Get() がエラーとして返すと仮定
	if err != nil {
		return nil, fmt.Errorf("/speakers API呼び出し失敗。VOICEVOXエンジンが起動しているか確認してください: %w", err)
	}

	// 変更点3: ステータスコードチェックとエラーボディ読み込みのロジックを削除

	var vvSpeakers []VVSpeaker
	// 変更点4: bodyBytes から直接 JSON デコード（Unmarshal）
	if err := json.Unmarshal(bodyBytes, &vvSpeakers); err != nil {
		// JSON デコード失敗時のデバッグ情報
		bodyDisplay := string(bodyBytes)
		if len(bodyDisplay) > 100 {
			bodyDisplay = bodyDisplay[:100] + "..."
		}
		return nil, fmt.Errorf("/speakers 応答のJSONデコード失敗。返されたボディ: %s。エラー: %w", bodyDisplay, err)
	}

	data := &SpeakerData{
		StyleIDMap:       make(map[string]int),
		DefaultStyleMap:  make(map[string]string),
		apiNameToToolTag: apiNameToToolTag, // 構築した内部マップを保持
	}

	// 応答データから StyleIDMap と DefaultStyleMap を構築
	for _, spk := range vvSpeakers {
		// 内部マップ 'apiNameToToolTag' を使用
		toolTag, tagFound := apiNameToToolTag[spk.Name]

		if !tagFound {
			continue
		}

		for _, style := range spk.Styles {
			styleTag, tagExists := styleApiNameToToolTag[style.Name]
			if !tagExists {
				slog.Debug("サポートされていないスタイルをスキップします", "speaker", spk.Name, "style", style.Name)
				continue
			}

			combinedTag := toolTag + styleTag // 例: "[めたん][ノーマル]"
			data.StyleIDMap[combinedTag] = style.ID

			// VvTagNormal ([ノーマル]) スタイルをデフォルトとして登録
			if styleTag == VvTagNormal {
				data.DefaultStyleMap[toolTag] = combinedTag
			}
		}
	}

	// 必須のデフォルトスタイルが存在するかチェック (SupportedSpeakersのすべてのエントリについてチェック)
	missingDefaults := []string{}
	for _, mapping := range SupportedSpeakers {
		toolTag := mapping.ToolTag
		if _, ok := data.DefaultStyleMap[toolTag]; !ok {
			slog.Error("必須話者のデフォルトスタイルが見つかりません", "speaker", toolTag, "required_style", VvTagNormal)
			missingDefaults = append(missingDefaults, mapping.APIName)
		}
	}

	if len(missingDefaults) > 0 {
		return nil, fmt.Errorf("VOICEVOXエンジンに以下の必須話者またはそのデフォルトスタイル（%s）がありません: %s", VvTagNormal, strings.Join(missingDefaults, ", "))
	}

	slog.Info("VOICEVOXスタイルデータが正常にロードされました", "styles_count", len(data.StyleIDMap))

	return data, nil
}
