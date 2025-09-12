package wordlist

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
)

type WordlistSplitter struct {
	TempDir string
}

type WordlistChunk struct {
	FilePath   string
	StartLine  int64
	EndLine    int64
	ChunkIndex int
}

func NewWordlistSplitter(tempDir string) *WordlistSplitter {
	return &WordlistSplitter{
		TempDir: tempDir,
	}
}

// CountLines counts the total number of lines in a wordlist file
func (ws *WordlistSplitter) CountLines(wordlistPath string) (int64, error) {
	file, err := os.Open(wordlistPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var lineCount int64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineCount++
	}

	return lineCount, scanner.Err()
}

// SplitWordlist splits a wordlist into equal chunks for multiple workers
func (ws *WordlistSplitter) SplitWordlist(wordlistPath string, jobUID string, numWorkers int) ([]WordlistChunk, error) {
	if numWorkers <= 0 {
		return nil, fmt.Errorf("number of workers must be greater than 0")
	}

	// Count total lines in the wordlist
	totalLines, err := ws.CountLines(wordlistPath)
	if err != nil {
		return nil, fmt.Errorf("failed to count lines: %w", err)
	}

	if totalLines == 0 {
		return nil, fmt.Errorf("wordlist is empty")
	}

	// Calculate lines per chunk
	linesPerChunk := totalLines / int64(numWorkers)
	remainder := totalLines % int64(numWorkers)

	chunks := make([]WordlistChunk, numWorkers)
	currentLine := int64(0)

	// Open the source wordlist
	sourceFile, err := os.Open(wordlistPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open wordlist: %w", err)
	}
	defer sourceFile.Close()

	scanner := bufio.NewScanner(sourceFile)

	for i := 0; i < numWorkers; i++ {
		chunkSize := linesPerChunk
		// Distribute remainder lines to first few chunks
		if int64(i) < remainder {
			chunkSize++
		}

		// Skip empty chunks
		if chunkSize == 0 {
			continue
		}

		// Create chunk file
		chunkPath := filepath.Join(ws.TempDir, fmt.Sprintf("%s_chunk_%d.wordlist", jobUID, i))
		chunkFile, err := os.Create(chunkPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create chunk file: %w", err)
		}

		writer := bufio.NewWriter(chunkFile)

		// Write lines to chunk
		var linesWritten int64
		for linesWritten < chunkSize && scanner.Scan() {
			line := scanner.Text()
			writer.WriteString(line + "\n")
			linesWritten++
		}

		writer.Flush()
		chunkFile.Close()

		chunks[i] = WordlistChunk{
			FilePath:   chunkPath,
			StartLine:  currentLine,
			EndLine:    currentLine + linesWritten - 1,
			ChunkIndex: i,
		}

		currentLine += linesWritten
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading wordlist: %w", err)
	}

	return chunks, nil
}

// CleanupChunks removes all wordlist chunks for a specific job
func (ws *WordlistSplitter) CleanupChunks(jobUID string) error {
	pattern := filepath.Join(ws.TempDir, fmt.Sprintf("%s_chunk_*.wordlist", jobUID))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to find chunk files: %w", err)
	}

	for _, match := range matches {
		if err := os.Remove(match); err != nil {
			// Log error but continue cleanup
			fmt.Printf("Warning: failed to remove chunk file %s: %v\n", match, err)
		}
	}

	return nil
}