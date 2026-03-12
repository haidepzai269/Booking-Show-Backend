package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/gin-gonic/gin"
)

// ─── Semantic Cache Key Schema ─────────────────────────────────────────────────
// Mỗi entry cache trong Redis là một JSON struct chứa: câu hỏi gốc, vector, và câu trả lời.
// Key Redis: "chat:semantic:cache" là một JSON Array của SemanticCacheEntry.
// Để đơn giản và tránh phụ thuộc RedisSearch module (Không có Redis Stack), chúng ta
// lưu cache dưới dạng một chuỗi nhỏ các entry và tìm kiếm bằng cosine similarity trong Go.

const (
	semanticCacheKey       = "chat:semantic:cache"
	semanticCacheTTL       = 12 * time.Hour // Cache sống 12 tiếng
	similarityThreshold    = 0.85           // Ngưỡng 85% để coi là "cùng ý nghĩa"
	maxCacheEntries        = 200            // Giới hạn số lượng entry để không tốn RAM
	openclawRequestTimeout = 30 * time.Second
)

type SemanticCacheEntry struct {
	Question string    `json:"q"`
	Vector   []float32 `json:"v"`
	Answer   string    `json:"a"`
	CachedAt int64     `json:"ts"`
}

// ChatHandler xử lý mọi tương tác chatbot từ người dùng
type ChatHandler struct{}

func NewChatHandler() *ChatHandler {
	return &ChatHandler{}
}

// Chat là endpoint chính: POST /api/v1/chat
// Luồng: Nhận câu hỏi → Tạo Vector (HuggingFace) → Kiểm tra Semantic Cache (Redis)
// → Cache Hit: Trả về ngay → Cache Miss: Gọi OpenClaw → Lưu vào Cache → Trả về
func (h *ChatHandler) Chat(c *gin.Context) {
	var req struct {
		Question string `json:"question" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Vui lòng nhập câu hỏi hợp lệ"})
		return
	}

	question := req.Question

	// ─── 1. Tạo Vector từ câu hỏi (HuggingFace - Miễn phí) ─────────────────
	queryVector, embeddingErr := generateEmbedding(question)

	// ─── 2. Kiểm tra Semantic Cache trong Redis (chỉ khi có vector) ──────────
	if embeddingErr == nil && redispkg.Client != nil {
		if cachedAnswer, found := searchSemanticCache(queryVector); found {
			// Cache Hit! Trả về ngay không tốn token
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"answer": cachedAnswer, "from_cache": true}})
			return
		}
	}

	// ─── 3. Thử gọi OpenClaw Orchestrator (Bản 8B ổn định) ────────────────────
	fmt.Printf("[ChatHandler] [%s] Nhận câu hỏi: %s\n", time.Now().Format("15:04:05"), question)
	// Giờ chúng ta gọi thẳng Agent bookingshow (đã được cấu hình dùng 8B) để tránh Rate Limit
	answer, err := forwardToOpenClaw(question, "bookingshow")
	if err != nil {
		fmt.Printf("[ChatHandler] [%s] OpenClaw (8B) thất bại: %v — Đang thử fallback cuối cùng\n", time.Now().Format("15:04:05"), err)

		// Thử gọi thẳng Groq LLM (không qua OpenClaw) làm dự phòng cuối cùng
		answer, err = callGroqDirectly(question)
		if err != nil {
			fmt.Printf("[ChatHandler] [%s] Fallback Groq cuối cùng cũng lỗi: %v\n", time.Now().Format("15:04:05"), err)
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Trợ lý AI đang bận, xin thử lại sau."})
			return
		}
		fmt.Printf("[ChatHandler] [%s] Fallback Groq thành công\n", time.Now().Format("15:04:05"))
	} else {
		fmt.Printf("[ChatHandler] [%s] OpenClaw (8B) trả lời thành công\n", time.Now().Format("15:04:05"))
	}

	// ─── 4. Lưu kết quả vào Semantic Cache để dùng cho các lần sau ──────────
	if embeddingErr == nil && redispkg.Client != nil {
		go saveSemanticCache(question, queryVector, answer)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"answer": answer, "from_cache": false}})
}

// ─── Hàm tạo Vector Embedding từ HuggingFace (Cùng logic với ai_service.go) ──
func generateEmbedding(text string) ([]float32, error) {
	url := "https://api-inference.huggingface.co/pipeline/feature-extraction/sentence-transformers/all-MiniLM-L6-v2"
	payload := map[string]interface{}{"inputs": text}
	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if hfToken := os.Getenv("HUGGINGFACE_TOKEN"); hfToken != "" {
		req.Header.Set("Authorization", "Bearer "+hfToken)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("huggingface request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("huggingface API returned status %d", resp.StatusCode)
	}

	var vector []float32
	if err := json.NewDecoder(resp.Body).Decode(&vector); err != nil {
		return nil, fmt.Errorf("failed to parse embedding: %v", err)
	}
	return vector, nil
}

// ─── Tính Cosine Similarity giữa 2 vector ─────────────────────────────────────
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// ─── Tìm kiếm trong Semantic Cache ───────────────────────────────────────────
func searchSemanticCache(queryVector []float32) (string, bool) {
	ctx := context.Background()
	cached, err := redispkg.Client.Get(ctx, semanticCacheKey).Result()
	if err != nil {
		return "", false
	}

	var entries []SemanticCacheEntry
	if err := json.Unmarshal([]byte(cached), &entries); err != nil {
		return "", false
	}

	for _, entry := range entries {
		sim := cosineSimilarity(queryVector, entry.Vector)
		if sim >= similarityThreshold {
			return entry.Answer, true
		}
	}
	return "", false
}

// ─── Lưu entry mới vào Semantic Cache Redis ───────────────────────────────────
func saveSemanticCache(question string, vector []float32, answer string) {
	ctx := context.Background()

	var entries []SemanticCacheEntry
	if cached, err := redispkg.Client.Get(ctx, semanticCacheKey).Result(); err == nil {
		_ = json.Unmarshal([]byte(cached), &entries)
	}

	// Thêm entry mới + giới hạn kích thước cache
	entries = append(entries, SemanticCacheEntry{
		Question: question,
		Vector:   vector,
		Answer:   answer,
		CachedAt: time.Now().Unix(),
	})
	if len(entries) > maxCacheEntries {
		entries = entries[len(entries)-maxCacheEntries:] // Giữ entries mới nhất
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return
	}
	_ = redispkg.Client.Set(ctx, semanticCacheKey, data, semanticCacheTTL).Err()
}

// ─── Forward câu hỏi sang OpenClaw Orchestrator ───────────────────────────────
// ─── Hàm kiểm tra nếu nội dung chứa thông báo Rate Limit ─────────────────────
func isRateLimitMessage(content string) bool {
	content = strings.ToLower(content)
	indicators := []string{
		"rate limit reached",
		"please try again later",
		"too many requests",
		"try again",
		"⚠️ api rate limit",
	}
	for _, indicator := range indicators {
		if strings.Contains(content, indicator) {
			return true
		}
	}
	return false
}

// ─── Forward câu hỏi sang OpenClaw Orchestrator (Kèm Retry) ───────────────────
func forwardToOpenClaw(question string, agentID string) (string, error) {
	openclawURL := os.Getenv("OPENCLAW_URL")
	if openclawURL == "" {
		openclawURL = "http://localhost:18789"
	}

	targetURL := fmt.Sprintf("%s/v1/chat/completions", openclawURL)
	payload := map[string]interface{}{
		"model": agentID,
		"messages": []map[string]string{
			{"role": "user", "content": question},
		},
		"stream": false,
	}
	jsonData, _ := json.Marshal(payload)

	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			waitSec := time.Duration(i*2) * time.Second
			fmt.Printf("[ChatHandler] [%s] Lần thử %d: Chờ %v trước khi gọi lại OpenClaw...\n", time.Now().Format("15:04:05"), i+1, waitSec)
			time.Sleep(waitSec)
		}

		fmt.Printf("[ChatHandler] [%s] [Lần %d] Gọi OpenClaw: %s\n", time.Now().Format("15:04:05"), i+1, targetURL)

		req, err := http.NewRequestWithContext(
			context.Background(),
			"POST",
			targetURL,
			bytes.NewBuffer(jsonData),
		)
		if err != nil {
			return "", fmt.Errorf("failed to create openclaw request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+os.Getenv("OPENCLAW_API_KEY"))

		client := &http.Client{Timeout: openclawRequestTimeout}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("connection error: %v", err)
			fmt.Printf("[ChatHandler] [%s] Lỗi kết nối OpenClaw: %v\n", time.Now().Format("15:04:05"), err)
			continue
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read body: %v", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyBytes))
			fmt.Printf("[ChatHandler] [%s] OpenClaw trả về lỗi HTTP %d\n", time.Now().Format("15:04:05"), resp.StatusCode)
			continue
		}

		var result map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &result); err != nil {
			lastErr = fmt.Errorf("json parse error: %v", err)
			continue
		}

		// Lấy content
		var content string
		if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if msg, ok := choice["message"].(map[string]interface{}); ok {
					if c, ok := msg["content"].(string); ok {
						content = c
					}
				}
			}
		}

		if content == "" {
			lastErr = fmt.Errorf("empty content in response")
			continue
		}

		// Kiểm tra Rate Limit trong nội dung
		if isRateLimitMessage(content) {
			lastErr = fmt.Errorf("content level rate limit detected: %s", content)
			fmt.Printf("[ChatHandler] [%s] Phát hiện Rate Limit trong text: %s\n", time.Now().Format("15:04:05"), content)
			continue
		}

		// Nếu tất cả ổn
		fmt.Printf("[ChatHandler] [%s] OpenClaw trả lời thành công sau %d lần thử\n", time.Now().Format("15:04:05"), i+1)
		return content, nil
	}

	return "", fmt.Errorf("openclaw failed after %d retries. Last error: %v", maxRetries, lastErr)
}

func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ─── Fallback: Gọi thẳng Groq API khi OpenClaw không hoạt động ──────────────
func callGroqDirectly(question string) (string, error) {
	groqAPIKey := os.Getenv("GROQ_API_KEY")
	if groqAPIKey == "" {
		return "", fmt.Errorf("GROQ_API_KEY chưa được cấu hình")
	}

	payload := map[string]interface{}{
		"model": "llama-3.1-8b-instant",
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "Bạn là trợ lý AI của rạp chiếu phim BookingShow. Hãy trả lời bằng tiếng Việt, thân thiện và ngắn gọn. Bạn có thể giúp khách hàng về: lịch chiếu phim, đặt vé, giá vé, khuyến mãi, thông tin rạp, và các câu hỏi liên quan đến dịch vụ rạp chiếu phim.",
			},
			{
				"role":    "user",
				"content": question,
			},
		},
		"temperature": 0.7,
		"max_tokens":  512,
	}

	jsonData, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(
		context.Background(),
		"POST",
		"https://api.groq.com/openai/v1/chat/completions",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create groq request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+groqAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("groq request failed: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read groq response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("groq returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", fmt.Errorf("failed to parse groq response: %v", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("groq returned empty choices")
	}

	return result.Choices[0].Message.Content, nil
}
