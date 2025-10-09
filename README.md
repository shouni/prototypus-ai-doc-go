# ✍️ prototypus-ai-doc-go

[![Language](https://img.shields.io/badge/Language-Go-blue)](https://golang.org/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/git-gemini-reviewer-go)](https://golang.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

# Prototypus AI Doc Go

**Prototypus AI Doc Go** は、Google Gemini API を使用して、開発ドキュメントや技術記事などの長い文章を、対話形式またはモノローグ形式のナレーションスクリプトに変換するための CLI ツールです。

## ✨ 主な機能

1.  **AIによる自動スクリプト生成**: 長文を対話（ずんだもん・四国めたん）またはモノローグ（ずんだもん）形式のスクリプトに変換します。
2.  **VOICEVOX連携（NEW!）**: 生成されたスクリプトをローカルで起動しているVOICEVOXエンジンに送信し、**連結された一つのWAV音声ファイル**として出力できます。
3.  **柔軟なI/O**: ファイル入力/出力、標準入力/出力、そして外部APIへの投稿に対応しています。
4.  **高い保守性**: Goのベストプラクティスに基づき、AI、I/O、VOICEVOX処理などのロジックが `internal` パッケージに分離されています。

---

## 📦 使い方

### 1. 環境設定

ツールを実行する前に、以下の環境変数を設定してください。

| 変数名 | 必須/任意 | 説明 |
| :--- | :--- | :--- |
| `GEMINI_API_KEY` | 必須 | Google AI Studio で取得した Gemini API キー。 |
| `VOICEVOX_API_URL` | VOICEVOX使用時必須 | ローカルで起動しているVOICEVOXエンジンのURL。 (例: `http://localhost:50021`) |
| `POST_API_URL` | 外部API投稿時必須 | スクリプトを投稿する外部APIのエンドポイント。 |

### 2. スクリプト生成コマンド

メインコマンドは `generate` です。

```bash
prototypus-ai-doc generate [flags]
````

#### フラグ一覧

| フラグ | 短縮形 | 説明 |
| :--- | :--- | :--- |
| `--input-file` | `-i` | 元となる文章ファイル (`.txt` や `.md`) のパス。省略時は標準入力を使用。 |
| `--output-file` | `-o` | 生成されたスクリプトを保存するファイル名。省略時は標準出力に出力。 |
| `--mode` | `-m` | スクリプトの形式: `dialogue` (対話) または `solo` (モノローグ)。(Default: `dialogue`) |
| `--voicevox` | `-v` | **(新機能)** 生成されたスクリプトをVOICEVOXで合成し、指定されたファイル名 (`.wav`) で保存します。**他の出力フラグと同時に指定できません。** |
| `--post-api` | `-p` | 生成されたスクリプトを `POST_API_URL` に投稿します。 |

-----

## 🔊 VOICEVOX連携の実行例

### 目的: この `README.md` を入力として使用し、対話スクリプトを生成、そして音声化する。

まず、この README ファイルを `README.md` として保存されていると仮定します。

#### 1\. VOICEVOXエンジンを起動し、環境変数を設定

```bash
# VOICEVOXを起動後、URLを設定
export VOICEVOX_API_URL="http://localhost:50021"
export GEMINI_API_KEY="AIzaSy...your-key...021"
```

#### 2\. コマンド実行

**入力ファイル (`-i`)** として自身の `README.md` を指定し、**VOICEVOX出力 (`-v`)** を `readme_audio.wav` に指定します。

```bash
# README.md の内容を元に対話スクリプトを生成し、WAVファイルを出力
./bin/prototypus-ai-doc generate \
    -i README.md \
    -m dialogue \
    -v readme_audio.wav
```

#### 実行結果 (ターミナル出力)

```
ファイルから読み込み中: README.md
--- 処理開始 ---
モード: dialogue
モデル: gemini-2.5-flash
...（入力サイズなどの情報）...

AIによるスクリプト生成を開始します...

--- AI スクリプト生成完了 ---
VOICEVOXエンジンに接続し、音声合成を開始します (出力: readme_audio.wav)...
VOICEVOXによる音声合成が完了し、ファイルに保存されました。

# 現在のディレクトリに readme_audio.wav が生成されます。
```

-----

### 📜 ライセンス (License)

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。