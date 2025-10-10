package ioutils

import (
	"fmt"
	"io"
	"os"
)

// ReadInput は、ファイルまたは標準入力から内容を読み込みます。
func ReadInput(filename string) ([]byte, error) {
	if filename != "" {
		fmt.Fprintf(os.Stderr, "ファイルから読み込み中: %s\n", filename)
		return os.ReadFile(filename)
	}

	fmt.Fprintln(os.Stderr, "標準入力 (stdin) から読み込み中...")
	content, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("標準入力からの読み込みに失敗しました: %w", err)
	}
	return content, nil
}

// WriteOutput は、ファイルまたは標準出力に内容を書き出します。
func WriteOutput(filename string, content string) error {
	if filename != "" {
		fmt.Fprintf(os.Stderr, "\n--- スクリプト生成完了 ---\nファイルに書き込みました: %s\n", filename)
		return os.WriteFile(filename, []byte(content), 0644)
	}

	fmt.Fprintln(os.Stderr, "\n--- スクリプト生成結果 ---")
	// スクリプト本体は標準出力に出力 (パイプ処理を考慮)
	fmt.Fprintln(os.Stdout, content)
	return nil
}
