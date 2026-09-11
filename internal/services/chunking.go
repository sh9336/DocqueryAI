package services

import "strings"

// ChunkText splits a given text into smaller chunks of a specified size with a given overlap.
func ChunkText(text string, chunkSize int, chunkOverlap int) []string {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return nil
	}

	if chunkSize <= 0 {
		chunkSize = 1000
	}
	if chunkOverlap < 0 {
		chunkOverlap = 200
	}
	if chunkOverlap >= chunkSize {
		chunkOverlap = chunkSize / 2
	}

	// Operate on runes so multi-byte UTF-8 characters are never split mid-codepoint
	runes := []rune(text)
	total := len(runes)
	step := chunkSize - chunkOverlap

	var chunks []string
	for i := 0; i < total; i += step {
		end := i + chunkSize
		if end > total {
			end = total
		}
		chunks = append(chunks, string(runes[i:end]))
		if end == total {
			break
		}
	}

	return chunks
}
