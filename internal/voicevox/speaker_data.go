package voicevox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
func LoadSpeakers(ctx context.Context, client *Client) (*SpeakerData, error) {
	// 1. 静的なSupportedSpeakersから、内部使用のためのマップを構築
	apiNameToToolTag := make(map[string]string)
	for _, mapping := range SupportedSpeakers {
		apiNameToToolTag[mapping.APIName] = mapping.ToolTag
	}

	speakersURL := fmt.Sprintf("%s/speakers", client.apiURL)

	resp, err := client.Get(speakersURL, ctx)
	if err != nil {
		return nil, fmt.Errorf("/speakers API呼び出し失敗。VOICEVOXエンジンが起動しているか確認してください: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("/speakers APIがエラーを返しました: Status %d, Body: %s", resp.StatusCode, string(errorBody))
	}

	var vvSpeakers []VVSpeaker
	if err := json.NewDecoder(resp.Body).Decode(&vvSpeakers); err != nil {
		return nil, fmt.Errorf("/speakers 応答のJSONデコード失敗: %w", err)
	}

	data := &SpeakerData{
		StyleIDMap:       make(map[string]int),
		DefaultStyleMap:  make(map[string]string),
		apiNameToToolTag: apiNameToToolTag, // 構築した内部マップを保持
	}

	// 応答データから StyleIDMap と DefaultStyleMap を構築
	for _, spk := range vvSpeakers {
		// 変更点: 内部マップ 'apiNameToToolTag' を使用
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
			missingDefaults = append(missingDefaults, toolTag)
		}
	}

	if len(missingDefaults) > 0 {
		return nil, fmt.Errorf("VOICEVOXエンジンに以下の必須話者またはそのデフォルトスタイル（%s）がありません: %s", VvTagNormal, strings.Join(missingDefaults, ", "))
	}

	slog.Info("VOICEVOXスタイルデータが正常にロードされました", "styles_count", len(data.StyleIDMap))

	return data, nil
}
