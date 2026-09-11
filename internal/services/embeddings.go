package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// ErrNoContext signals that GenerateAnswer/OpenAnswerStream was asked a
// question with no retrieved context — the caller should show the standard
// "no documents found" message instead of making a Cohere call at all.
var ErrNoContext = errors.New("no context available to answer from")

var CohereAPIKey string

func InitOpenAI(apiKey string) {
	CohereAPIKey = apiKey
	log.Printf("[INIT] Using Cohere API key: %s...", apiKey[:min(8, len(apiKey))])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GenerateEmbeddings embeds texts with Cohere. inputType must be "search_document"
// for indexed content or "search_query" for user questions — embed-english-v3.0 is
// asymmetric, so mismatching these skews similarity search results.
func GenerateEmbeddings(ctx context.Context, texts []string, inputType string) ([][]float32, error) {
	if CohereAPIKey == "" {
		return nil, fmt.Errorf("Cohere API key is not initialized")
	}
	if err := checkBudget(embedLimiter); err != nil {
		return nil, err
	}

	const url = "https://api.cohere.ai/v1/embed"

	type Request struct {
		Texts          []string `json:"texts"`
		Model          string   `json:"model"`
		InputType      string   `json:"input_type"`
		EmbeddingTypes []string `json:"embedding_types"`
	}

	type Response struct {
		Embeddings struct {
			Float_ [][]float32 `json:"float"`
		} `json:"embeddings"`
	}

	reqData := Request{
		Texts:          texts,
		Model:          "embed-english-v3.0",
		InputType:      inputType,
		EmbeddingTypes: []string{"float"},
	}

	body, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+CohereAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, cohereStatusError(resp.StatusCode, string(respBody))
	}

	var respData Response
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return nil, err
	}

	if len(respData.Embeddings.Float_) == 0 {
		return nil, fmt.Errorf("empty embeddings returned from Cohere")
	}

	log.Printf("[EMBEDDINGS] Generated %d embeddings via Cohere", len(respData.Embeddings.Float_))
	return respData.Embeddings.Float_, nil
}

// OpenAnswerStream starts a streaming Cohere chat completion for question,
// grounded in contextText. If previousAnswer is non-empty, it's replayed
// back to Cohere as the assistant's own prior turn (with a follow-up user
// turn asking it to continue), so the model picks up exactly where a
// truncated answer left off instead of re-answering from scratch.
//
// Every failure mode — missing API key, local rate budget, empty question,
// no context, a non-2xx from Cohere — is resolved synchronously here,
// before a single byte reaches the caller. That matters because the caller
// (ask.go) wants to answer with a normal JSON error status for any of
// these, and can only do that if they surface *before* it commits to an
// SSE response. Only once Cohere has confirmed 200 does this function hand
// back a stream for PumpAnswerStream to relay live.
func OpenAnswerStream(ctx context.Context, contextText string, question string, previousAnswer string) (io.ReadCloser, error) {
	if CohereAPIKey == "" {
		return nil, fmt.Errorf("Cohere API key is not initialized")
	}
	if err := checkBudget(chatLimiter); err != nil {
		return nil, err
	}

	// Validate inputs
	contextText = strings.TrimSpace(contextText)
	question = strings.TrimSpace(question)
	previousAnswer = strings.TrimSpace(previousAnswer)

	if question == "" {
		return nil, fmt.Errorf("question cannot be empty")
	}

	if contextText == "" {
		return nil, ErrNoContext
	}

	const url = "https://api.cohere.ai/v2/chat"

	systemPrompt := `You are a helpful assistant that answers questions using only the provided context. If the answer is not in the context, say so plainly instead of making up an answer.

Formatting rules:
- Answer directly. Do not open with a restatement of the question or a phrase like "Below is..." or "Here is...".
- Never mention the context, documents, chunks, embeddings, retrieval, or your own internal process — just answer as if you know the material.
- Write in concise, natural paragraphs. Prefer prose over lists when the content isn't inherently a sequence or collection.
- Use numbered lists only for ordered steps or procedures.
- Use bullet lists only for a genuine collection of related, parallel items.
- Use Markdown headings only for long, multi-section answers — never for a short answer.
- Use **bold** sparingly, only for genuinely important terms.
- Use fenced code blocks (with a language tag, e.g. ` + "```" + `bash) for any command, source code, config, or JSON. Preserve exact line breaks, indentation, and quoting inside code blocks — never merge multiple lines or commands into one.
- Use a Markdown table only when the information is genuinely tabular (e.g. a comparison).
- Match the response's length and structure to the question: a simple question gets a short, direct answer; a procedural question gets steps; a comparison gets a table or short bullets; a long or multi-part question may use a few shallow headings, but avoid deep nesting.
- Do not repeat information or pad the answer to look thorough.`
	userPrompt := fmt.Sprintf("Context:\n%s\n\nQuestion: %s", contextText, question)

	type Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	type Request struct {
		Model       string    `json:"model"`
		Messages    []Message `json:"messages"`
		MaxTokens   int       `json:"max_tokens"`
		Temperature float32   `json:"temperature"`
		Stream      bool      `json:"stream"`
	}

	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	if previousAnswer != "" {
		// Replay the truncated answer as the assistant's own turn, then ask
		// it to pick up exactly where it left off — this is what lets a
		// max_tokens cutoff actually continue instead of re-answering.
		messages = append(messages,
			Message{Role: "assistant", Content: previousAnswer},
			Message{Role: "user", Content: "Continue your previous answer from exactly where it left off. Do not repeat anything already said, and do not add a new introduction or restart the answer."},
		)
	}

	log.Printf("[ANSWER] Context length: %d chars, Question: %q, System prompt: %d chars, continuing: %v", len(contextText), question, len(systemPrompt), previousAnswer != "")
	log.Printf("[ANSWER] User prompt length: %d chars", len(userPrompt))

	reqData := Request{
		Model:       "command-a-plus-05-2026",
		Messages:    messages,
		MaxTokens:   2048,
		Temperature: 0.8,
		Stream:      true,
	}

	bodyBytes, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal answer request: %w", err)
	}

	log.Printf("[ANSWER] Request body size: %d bytes, message content length: %d", len(bodyBytes), len(userPrompt))

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+CohereAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, cohereStatusError(resp.StatusCode, string(respBody))
	}

	log.Printf("[ANSWER] ✓ Stream connected")
	return resp.Body, nil
}

// answerStreamEvent mirrors the subset of Cohere v2 chat's SSE event shape
// PumpAnswerStream needs: a content-delta event carries one incremental
// chunk of text, and the terminal message-end event carries finish_reason.
type answerStreamEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Message struct {
			Content struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"delta"`
}

// PumpAnswerStream reads an SSE stream opened by OpenAnswerStream, calling
// onDelta with each chunk of answer text as it arrives, and returns the
// full assembled answer plus whether it was cut short by the token budget.
// It always closes body, even on error.
func PumpAnswerStream(body io.ReadCloser, onDelta func(string)) (string, bool, error) {
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var full strings.Builder
	truncated := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if line == "" {
			continue
		}

		var ev answerStreamEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue // heartbeats / non-JSON lines
		}

		switch ev.Type {
		case "content-delta":
			if text := ev.Delta.Message.Content.Text; text != "" {
				full.WriteString(text)
				if onDelta != nil {
					onDelta(text)
				}
			}
		case "message-end":
			truncated = ev.Delta.FinishReason == "MAX_TOKENS"
		}
	}

	answer := strings.TrimSpace(full.String())
	if err := scanner.Err(); err != nil {
		log.Printf("[ANSWER] ❌ Stream read error after %d chars: %v", len(answer), err)
		return answer, truncated, err
	}

	if truncated {
		log.Printf("[ANSWER] ⚠️ Answer truncated at max_tokens (%d chars)", len(answer))
	}
	log.Printf("[ANSWER] ✓ Stream complete (%d chars)", len(answer))
	return answer, truncated, nil
}
