package voicevox

import (
	"encoding/binary"
	"fmt"
)

// ----------------------------------------------------------------------
// 定数 (WAVヘッダーサイズ)
// ----------------------------------------------------------------------

const (
	RiffChunkIDSize    = 4                                                 // "RIFF"
	RiffChunkSizeField = 4                                                 // ファイルサイズ - 8
	WaveIDSize         = 4                                                 // "WAVE"
	WavRiffHeaderSize  = RiffChunkIDSize + RiffChunkSizeField + WaveIDSize // 12 bytes

	FmtChunkIDSize    = 4                                                     // "fmt "
	FmtChunkSizeField = 4                                                     // 16 (固定)
	FmtChunkDataSize  = 16                                                    // フォーマット情報
	WavFmtChunkSize   = FmtChunkIDSize + FmtChunkSizeField + FmtChunkDataSize // 24 bytes

	DataChunkIDSize    = 4                                    // "data"
	DataChunkSizeField = 4                                    // データサイズ (PCMデータ長)
	WavDataHeaderSize  = DataChunkIDSize + DataChunkSizeField // 8 bytes

	// WAVヘッダー全体のサイズ (RIFF + FMT + DATA ヘッダー)
	WavTotalHeaderSize = WavRiffHeaderSize + WavFmtChunkSize + WavDataHeaderSize // 44 bytes
)

// ----------------------------------------------------------------------
// ヘルパー関数
// ----------------------------------------------------------------------

// extractAudioData は単一のWAVファイルバイトスライスからオーディオデータ部分とサイズを抽出します。
func extractAudioData(wavBytes []byte, index int) ([]byte, uint32, error) {
	if len(wavBytes) < WavTotalHeaderSize {
		return nil, 0, fmt.Errorf("WAVファイル #%d のヘッダーが短すぎます (最低 %dバイト必要)", index, WavTotalHeaderSize)
	}

	// Data Chunk Size (データチャンクのサイズフィールドは全体ヘッダーの末尾に位置)
	dataSizeStartIndex := WavTotalHeaderSize - DataChunkSizeField
	dataSize := binary.LittleEndian.Uint32(wavBytes[dataSizeStartIndex:WavTotalHeaderSize])

	// Data Chunk の実際のオーディオデータを抽出する際の境界チェック
	dataEndIndex := WavTotalHeaderSize + dataSize
	if uint32(len(wavBytes)) < dataEndIndex {
		return nil, 0, fmt.Errorf("WAVファイル #%d のデータ長がヘッダーの記載と一致しません (記載: %d, 実際: %d)",
			index, dataSize, len(wavBytes)-WavTotalHeaderSize)
	}

	// Data Chunk の実際のオーディオデータを抽出
	dataChunk := wavBytes[WavTotalHeaderSize:dataEndIndex]

	return dataChunk, dataSize, nil
}

// buildCombinedWav はフォーマットヘッダー情報と結合されたオーディオデータから、
// 正しいヘッダーを持つ単一のWAVファイルを構築します。
func buildCombinedWav(formatHeader []byte, rawData []byte, totalDataSize uint32) []byte {
	// 結合後の最終WAVバイトスライスを準備
	combinedWav := make([]byte, WavTotalHeaderSize+totalDataSize)

	// 1. フォーマットヘッダーをコピー (RIFF ID, RIFF Size, WAVE ID, FMT Chunk)
	// formatHeaderはRIFF IDからFMT Dataまで(36バイト)を含む
	copy(combinedWav, formatHeader)

	// 2. RIFF Chunk Size (ファイルサイズ - 8) を書き込む
	// RIFF Chunk Size = WAVE ID以降の全データ長
	riffChunkDataSize := WaveIDSize + WavFmtChunkSize + WavDataHeaderSize + totalDataSize
	fileSizeStartIndex := RiffChunkIDSize // RIFF ID("RIFF")の直後
	binary.LittleEndian.PutUint32(combinedWav[fileSizeStartIndex:fileSizeStartIndex+RiffChunkSizeField], riffChunkDataSize)

	// 3. Data Chunk の ID ("data") を書き込む
	dataIDStartIndex := WavRiffHeaderSize + WavFmtChunkSize
	copy(combinedWav[dataIDStartIndex:dataIDStartIndex+DataChunkIDSize], []byte("data"))

	// 4. Data Chunk Size (総オーディオデータ長) を書き込む
	dataSizeStartIndex := WavTotalHeaderSize - DataChunkSizeField
	binary.LittleEndian.PutUint32(combinedWav[dataSizeStartIndex:WavTotalHeaderSize], totalDataSize)

	// 5. 結合したオーディオデータを追加
	copy(combinedWav[WavTotalHeaderSize:], rawData)

	return combinedWav
}

// ----------------------------------------------------------------------
// メインロジック
// ----------------------------------------------------------------------

// combineWavData は複数のWAVデータのバイトスライスを受け取り、
// それらのオーディオデータ部分を連結し、新しい正しいヘッダーを持つ単一のWAVファイルを生成します。
func combineWavData(wavFiles [][]byte) ([]byte, error) {
	if len(wavFiles) == 0 {
		return nil, fmt.Errorf("結合するWAVデータがありません")
	}

	// 最初のファイルからフォーマット情報（RIFF ID, RIFF Size, WAVE ID, FMT Chunk）を抽出
	fmtChunkEndIndex := WavRiffHeaderSize + WavFmtChunkSize // 36バイト
	if len(wavFiles[0]) < fmtChunkEndIndex {
		return nil, fmt.Errorf("最初のWAVファイルのヘッダー（RIFF + FMT）が短すぎます (最低 %dバイト必要)", fmtChunkEndIndex)
	}
	// formatHeader: RIFF ID から FMT チャンクの終わりまで (36 bytes)
	formatHeader := wavFiles[0][0:fmtChunkEndIndex]

	var rawData []byte
	var totalDataSize uint32 = 0

	// 各WAVファイルからオーディオデータ部分を抽出して連結
	for i, wavBytes := range wavFiles {
		dataChunk, dataSize, err := extractAudioData(wavBytes, i)
		if err != nil {
			return nil, err
		}
		rawData = append(rawData, dataChunk...)
		totalDataSize += dataSize
	}

	if totalDataSize == 0 {
		// タイトルはあっても、本文データが空だった場合のチェック
		return nil, fmt.Errorf("すべてのWAVファイルから抽出されたオーディオデータがゼロサイズです")
	}

	// 新しいヘッダーを作成し、結合したオーディオデータを格納
	return buildCombinedWav(formatHeader, rawData, totalDataSize), nil
}
