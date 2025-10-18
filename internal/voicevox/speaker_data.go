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
// 話者タグとVOICEVOXスタイルタグの定数定義
// ----------------------------------------------------------------------

const (
	// AIプロンプトで使用する短縮タグ
	SpeakerTagZundamon = "[ずんだもん]"
	SpeakerTagMetan    = "[めたん]"
)

const (
	// VOICEVOXのスタイル名と一致させる定数
	VvTagNormal   = "[ノーマル]"
	VvTagAmaama   = "[あまあま]"
	VvTagTsuntsun = "[ツンツン]"
	VvTagSexy     = "[セクシー]"
	VvTagWhisper  = "[ささやき]" // ささやきタグ
	// 必要に応じて、さらに "ヒソヒソ", "ヘロヘロ", "なみだめ" などを追加可能
)

// VOICEVOX APIで使われる名前を、ツールの内部タグに変換するためのマッピング
var apiNameToToolTag = map[string]string{
	"四国めたん": SpeakerTagMetan,    // VOICEVOX API "四国めたん" -> ツールタグ "[めたん]"
	"ずんだもん": SpeakerTagZundamon, // VOICEVOX API "ずんだもん" -> ツールタグ "[ずんだもん]"
}

// ★ 修正: VOICEVOX APIのスタイル名からツールのタグ定数へのマッピングを追加
// これにより、VvTagAmaamaなどの定数が LoadSpeakers で利用される
var styleApiNameToToolTag = map[string]string{
	"ノーマル": VvTagNormal,
	"あまあま": VvTagAmaama,
	"ツンツン": VvTagTsuntsun,
	"セクシー": VvTagSexy,
	"ささやき": VvTagWhisper,
	// 必要に応じて、他のスタイル名（例: "あんぐり" -> VvTagAngry）もここに追加する
}

// ----------------------------------------------------------------------
// スタイルIDの動的データ構造とロードロジック
// ----------------------------------------------------------------------

// SpeakerData はVOICEVOXから動的に取得した全話者・スタイル情報を保持する
type SpeakerData struct {
	StyleIDMap      map[string]int    // 例: "[めたん][ノーマル]" -> 2
	DefaultStyleMap map[string]string // 例: "[めたん]" -> "[めたん][ノーマル]" (フォールバック用)
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
func LoadSpeakers(ctx context.Context, apiURL string) (*SpeakerData, error) {
	client := NewClient(apiURL)

	resp, err := client.Get(fmt.Sprintf("%s/speakers", client.apiURL), ctx)
	if err != nil {
		return nil, fmt.Errorf("/speakers API呼び出し失敗。VOICEVOXエンジンが起動しているか確認してください: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("/speakers APIがエラーを返しました: Status %d, Body: %s", resp.StatusCode, string(errorBody))
	}

	var vvSpeakers []VVSpeaker
	if err := json.NewDecoder(resp.Body).Decode(&vvSpeakers); err != nil {
		return nil, fmt.Errorf("/speakers 応答のJSONデコード失敗: %w", err)
	}

	data := &SpeakerData{
		StyleIDMap:      make(map[string]int),
		DefaultStyleMap: make(map[string]string),
	}

	// 応答データから StyleIDMap と DefaultStyleMap を構築
	for _, spk := range vvSpeakers {
		// API名 ("四国めたん"など) から、ツールのタグ ("[めたん]"など) を取得
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

	// 必須のデフォルトスタイルが存在するかチェック (apiNameToToolTagのすべての値についてチェック)
	missingDefaults := []string{}
	for _, tag := range apiNameToToolTag {
		if _, ok := data.DefaultStyleMap[tag]; !ok {
			// エラーログを出力
			slog.Error("必須話者のデフォルトスタイルが見つかりません", "speaker", tag, "required_style", VvTagNormal)
			missingDefaults = append(missingDefaults, tag)
		}
	}

	if len(missingDefaults) > 0 {
		return nil, fmt.Errorf("VOICEVOXエンジンに以下の必須話者またはそのデフォルトスタイル（%s）がありません: %s", VvTagNormal, strings.Join(missingDefaults, ", "))
	}

	slog.Info("VOICEVOXスタイルデータが正常にロードされました", "styles_count", len(data.StyleIDMap))

	return data, nil
}
