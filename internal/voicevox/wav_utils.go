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
// WAVデータ結合ロジック
// ----------------------------------------------------------------------

// combineWavData は複数のWAVデータのバイトスライスを受け取り、
// それらのオーディオデータ部分を連結し、新しい正しいヘッダーを持つ単一のWAVファイルを生成します。
func combineWavData(wavFiles [][]byte) ([]byte, error) {
	if len(wavFiles) == 0 {
		return nil, fmt.Errorf("結合するWAVデータがありません")
	}

	var rawData []byte
	totalDataSize := uint32(0)

	// 最初のファイルからフォーマット情報（RIFFとFMTチャンク）を抽出
	fmtChunkEndIndex := WavRiffHeaderSize + WavFmtChunkSize
	if len(wavFiles[0]) < fmtChunkEndIndex {
		return nil, fmt.Errorf("最初のWAVファイルのヘッダー（RIFF + FMT）が短すぎます")
	}
	// RIFFチャンクIDからFMTチャンクの終わりまで（RIFFサイズフィールドは含まないが、上書きするのでOK）
	formatHeader := wavFiles[0][0:fmtChunkEndIndex]

	for i, wavBytes := range wavFiles {
		if len(wavBytes) < WavTotalHeaderSize {
			return nil, fmt.Errorf("WAVファイル #%d が完全なヘッダーを含んでいません", i)
		}

		// WAVデータのサイズ（Data Chunk Size）を読み取る
		dataSizeStartIndex := WavTotalHeaderSize - DataChunkSizeField
		dataSize := binary.LittleEndian.Uint32(wavBytes[dataSizeStartIndex:WavTotalHeaderSize])

		// Data Chunk の実際のオーディオデータを抽出
		dataChunk := wavBytes[WavTotalHeaderSize : WavTotalHeaderSize+dataSize]

		rawData = append(rawData, dataChunk...)
		totalDataSize += dataSize
	}

	// 結合後の最終WAVバイトスライスを準備
	combinedWav := make([]byte, WavTotalHeaderSize+totalDataSize)

	// 1. フォーマットヘッダーをコピー (RIFF ID, WAVE ID, FMT Chunk)
	copy(combinedWav, formatHeader)

	// 2. RIFF Chunk Size (ファイルサイズ - 8) を書き込む
	fileSize := WavTotalHeaderSize + totalDataSize - RiffChunkIDSize + RiffChunkSizeField
	fileSizeStartIndex := RiffChunkIDSize // RIFF ID("RIFF")の直後
	binary.LittleEndian.PutUint32(combinedWav[fileSizeStartIndex:fileSizeStartIndex+RiffChunkSizeField], fileSize)

	// 3. Data Chunk の ID ("data") を書き込む
	dataIDStartIndex := WavRiffHeaderSize + WavFmtChunkSize
	copy(combinedWav[dataIDStartIndex:], []byte("data"))

	// 4. Data Chunk Size (総オーディオデータ長) を書き込む
	dataSizeStartIndex := WavTotalHeaderSize - DataChunkSizeField
	binary.LittleEndian.PutUint32(combinedWav[dataSizeStartIndex:WavTotalHeaderSize], totalDataSize)

	// 5. 結合したオーディオデータを追加
	copy(combinedWav[WavTotalHeaderSize:], rawData)

	return combinedWav, nil
}
