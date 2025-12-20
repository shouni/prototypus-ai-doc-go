# ✍️ Prototypus AI Doc Go

[![Language](https://img.shields.io/badge/Language-Go-blue)](https://golang.org/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/prototypus-ai-doc-go)](https://golang.org/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/prototypus-ai-doc-go)](https://github.com/shouni/prototypus-ai-doc-go/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## 💡 概要 (About)— **堅牢なGo並列処理とAIを統合した次世代ドキュメント音声化パイプライン**

**Prototypus AI Doc Go (PAID Go)** は、独自の **Gemini API クライアントライブラリ** [`shouni/go-ai-client`](https://github.com/shouni/go-ai-client) と **Go言語の強力な並列制御**を融合させた、**業界最高水準の堅牢性**を持つ高生産性 CLI ツールです。

長文の技術ドキュメントやWeb記事を、AIが話者とスタイルを明確に指示した**対話形式やモノローグ形式のナレーションスクリプト**に変換するだけでなく、その台本をローカルの **VOICEVOXエンジンに高速接続**し、**最終的な音声ファイル (WAV)** を生成します。
このツールは、単なるスクリプト生成を超え、Go言語の**堅牢な並列制御とデータ整合性チェック**により、コンテンツ制作の全工程を自動化し、開発チームのドキュメント配信パイプラインに革新をもたらします。
**マルチプロトコル I/O** をサポート。入力ソースとして **Web URL**、**ローカルファイル**、**GCS (`gs://`)**、**Amazon S3 (`s3://`)** を透過的に扱うことができ、出力先もクラウドストレージへ直接保存可能です。

### 🌸 導入がもたらすポジティブな変化

| メリット | チームへの影響 | 期待される効果 |
| --- | --- | --- |
| **AIによる台本自動生成** | **「文章化の最初の壁」が解消します。** AIが複雑なドキュメントの校正やナレーション形式への変換を担うため、クリエイターは本質的な調整に集中できます。 | 制作にかかる時間が**最大80%短縮**され、コンテンツ制作のサイクルが劇的に加速します。 |
| **URL・マルチクラウド対応** | **「情報のコピペ」が不要になります。** 公開URLやクラウド（GCS/S3）上のファイルを直接指定して音声化できるため、ワークフローが極めてシンプルになります。 | データの移動コストがなくなり、開発チームの運用効率が飛躍的に向上します。 |
| **超高速・安定した音声合成** | **「結合の不整合」の心配が不要です。** Go言語の並列処理とリトライロジックにより、長文の音声合成も高速かつ高い成功率で完結します。 | 処理の安定性が保証され、大規模なドキュメントの音声化における技術的な労力から解放されます。 |
| **統合パイプラインの提供** | **「複数ツールの連携」という間接作業が不要です。** スクリプト生成からWAV出力までを一貫したCLIで完結させる統合環境を提供します。 | 制作の全工程が自動化され、開発者はドキュメント配信をストレスフリーで行えるようになります。 |

---

## ✨ 技術スタック (Technology Stack)

| 要素 | 技術 / ライブラリ | 役割 |
| --- | --- | --- |
| **言語** | **Go (Golang)** | クロスプラットフォームでの高速な実行を実現。 |
| **並行処理** | **Goroutines (`sync` パッケージ)** | 複数の音声合成リクエストを**並列**で実行し、処理時間を大幅に短縮。 |
| **AI モデル** | **Google Gemini API** | 独自のクライアントを通じ、入力文章を高度に分析・スクリプト化。 |
| **リモート I/O** | **`shouni/go-remote-io`** | **GCS, S3, ローカル**への透過的な読み書きを管理。 |
| **コンテンツ抽出** | **`shouni/go-web-exact`** | URLから不要な広告要素を排除し、純粋な本文のみを特定して抽出。 |
| **ネットワーク** | **`shouni/go-http-kit`** | VOICEVOX API等との通信におけるリトライとタイムアウトを堅牢に制御。 |

---

## 🛡️ 入出力とVOICEVOX連携の堅牢性

### 1. 多彩な入力ソースと統合 I/O 管理

本ツールは `shouni/go-remote-io` をベースとした `UniversalInputReader` を採用しています。

* **Webコンテンツ抽出**: `--script-url (-u)` を指定すると、Webサイトの構造を解析し、タイトルと本文のみをクリーンに取得します。
* **クラウド・ローカル入力**: `--script-file (-f)` では、パスの接頭辞 (`gs://`, `s3://`) を自動判別し、クラウド上のドキュメントを直接ストリームとして読み込みます。

### 2. 超高速な並列処理 (High-Speed Parallel Processing)

スクリプトをセグメント（文）単位に分割し、**GoのGoroutine**を用いて並列に音声合成を行います。最大並列実行数を制御することでエンジンの過負荷を防ぎつつ、逐次処理と比較して合成時間を劇的に短縮します。

### 3. スタイルIDの自動フォールバック

AIが生成したスタイルタグが VOICEVOX 側で定義されていない場合、自動的にその話者の「ノーマル」スタイルにフォールバックします。これにより、AIの微細な表現のゆらぎによってパイプラインが停止することはありません。

---

## ✨ 主な機能

1. **Webからの自動抽出**: `--script-url (-u)` でURLを指定するだけで、記事内容をAIに最適化して渡します。
2. **マルチプロトコル入力**: `--script-file (-f)` で**ローカル**, **GCS**, **S3** から直接読み込み可能。
3. **AIスクリプト生成**: **`solo`**, **`dialogue`**, **`duet`** の3形式から選択可能。
4. **VOICEVOX並列合成**: 生成された台本を並列リクエストで高速にWAV化し、連結して出力。
5. **堅牢なエラーハンドリング**: 指数バックオフによる自動リトライ、不正データ検出機能を完備。

---

## 📦 使い方

### 1. 環境設定

| 変数名 | 必須/任意 | 説明 |
| --- | --- | --- |
| `GEMINI_API_KEY` | 必須 | Google AI Studio で取得した API キー。 |
| `VOICEVOX_API_URL` | VOICEVOX使用時 | エンジンのURL (例: `http://localhost:50021`)。 |
| `GOOGLE_APPLICATION_CREDENTIALS` | GCS使用時 | GCS権限を持つサービスアカウントのJSONパス。 |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | S3使用時 | S3へのアクセス権限を持つ認証情報。 |

### 2. スクリプト生成コマンド

```bash
paidgo generate [flags]

```

#### フラグ一覧（入力ソースはいずれか一つを指定）

| フラグ | 短縮形 | 説明 |
| --- | --- | --- |
| `--script-url` | `-u` | **入力ソースURL**。Webから記事本文を抽出してAIに渡します。 |
| `--script-file` | `-f` | **入力ソースパス**。ローカル、**`gs://`**、**`s3://`**、または `'-'` (stdin)。 |
| `--output-file` | `-o` | 生成スクリプトの保存先。省略時は標準出力。 |
| `--mode` | `-m` | 形式: **`solo`**, **`dialogue`**, **`duet`** (Default: `duet`)。 |
| `--voicevox` | `-v` | 音声WAVの保存先 (例: `out.wav`, **`gs://bucket/out.wav`**)。 |
| `--http-timeout` |  | Webリクエストや合成のタイムアウト時間。 (Default: `60s`) |

---

## 🔊 実行例

### 例 1: Web記事を対話スクリプト化し、GCSに音声ファイルを出力（タイムアウト指定あり）

VOICEVOXエンジンとGCS認証（`GOOGLE_APPLICATION_CREDENTIALS`）が設定済みであることを前提とします。

```bash
# Web上の技術記事を読み込み、対話モードでスクリプト生成、生成されたWAVをGCSに直接アップロード
./bin/paidgo generate \
    --script-url "https://github.com/shouni/prototypus-ai-doc-go" \
    --mode dialogue \
    --http-timeout 280s \
    --voicevox gs://my-audio-bucket/docs/out_dialogue_20251116.wav
```

### 例 2: ローカルファイルをモノローグ化し、結果を画面に出力

```bash
# README.md の内容を元にモノローグスクリプトを生成し、標準出力に表示
./bin/paidgo generate \
    --script-file README.md \
    --mode solo \
    --output-file -  # 標準出力への明示的な指定 (省略可能)
```

-----

### ⚖️ クレジット表記 (Credits)

本プロジェクトを利用して音声を生成・公開する際は、以下のソフトウェアおよびキャラクターのライセンス・利用規約に従ってください。

### ソフトウェア

* **VOICEVOX**: [https://voicevox.hiroshiba.jp/](https://voicevox.hiroshiba.jp/)

### キャラクター

本プロジェクトのデフォルト設定やプロンプト例では、以下のキャラクターを利用しています。

* **VOICEVOX:ずんだもん**
* **VOICEVOX:四国めたん**

> **利用規約の遵守について**
> 音声を使用する際は、[VOICEVOX 利用規約](https://voicevox.hiroshiba.jp/term/)および、各キャラクターの利用規約（[ずんだもん・四国めたん 利用規約](https://zunko.jp/con_ongen_kiyaku.html)）を必ず確認し、適切なクレジット表記を行ってください。
> 例：`VOICEVOX:ずんだもん`、`VOICEVOX:四国めたん`

-----

### 📜 ライセンス (License)

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。
