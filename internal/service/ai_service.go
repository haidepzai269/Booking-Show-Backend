package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"
)

type AIService struct {
	groqAPIKey       string
	huggingFaceToken string
}

func NewAIService(groqKey, hfToken string) *AIService {
	// Nếu truyền vào trống, thử đọc từ env (để hỗ trợ các chỗ gọi chưa refactor)
	if groqKey == "" {
		groqKey = os.Getenv("GROQ_API_KEY")
	}
	if hfToken == "" {
		hfToken = os.Getenv("HUGGINGFACE_TOKEN")
	}

	return &AIService{
		groqAPIKey:       groqKey,
		huggingFaceToken: hfToken,
	}
}

// GroqRequest đại diện cho body request gửi lên Groq
type GroqRequest struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Temperature float32          `json:"temperature"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	ToolChoice  string           `json:"tool_choice,omitempty"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// GroqResponse đại diện cho dữ liệu trả về
type GroqResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

// AnalyzeSearchQuery nhận query của người dùng và danh sách phim (RAG metadata), trả về mảng IDs
func (s *AIService) AnalyzeSearchQuery(query string, moviesMeta string) ([]int, error) {
	if s.groqAPIKey == "" {
		return nil, fmt.Errorf("GROQ_API_KEY is not configured")
	}

	systemPrompt := `Bạn là một hệ thống AI Search (RAG) thông minh cho website đặt vé xem phim.
Nhiệm vụ của bạn là phân tích yêu cầu tìm kiếm của người dùng và đối chiếu với danh sách phim hiện có để trả về ID của các bộ phim phù hợp nhất.
Chỉ trả về JSON là một mảng các số nguyên đại diện cho các ID phim phù hợp. KHÔNG GIẢI THÍCH, KHÔNG CHỨA BẤT KỲ VĂN BẢN NÀO KHÁC NGOÀI JSON MẢNG (Ví dụ: [1, 5, 12]). 
Nếu không có phim nào phù hợp, hãy trả về mảng rỗng [].

Dưới đây là DANH SÁCH PHIM HIỆN CÓ (dữ liệu RAG):
` + moviesMeta

	userPrompt := "Yêu cầu tìm kiếm của người dùng: " + query

	reqBody := GroqRequest{
		Model: "llama-3.3-70b-versatile",
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.1, // Nhiệt độ thấp để câu trả lời chính xác, tránh hallucination
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.groqAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second} // Tăng timeout lên 10s để ổn định hơn
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Groq API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("groq api error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var groqRes GroqResponse
	if err := json.NewDecoder(resp.Body).Decode(&groqRes); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	if len(groqRes.Choices) == 0 {
		return []int{}, nil
	}

	// Lấy kết quả từ AI (dạng chuỗi mảng JSON)
	content := groqRes.Choices[0].Message.Content
	log.Printf("[Groq AI Debug] Raw Content: %s\n", content)

	var matchedIDs []int
	err = json.Unmarshal([]byte(content), &matchedIDs)
	if err != nil {
		// Thử lọc nếu AI có chen thêm text bằng Regex linh hoạt hơn
		re := regexp.MustCompile(`\[\s*\d*(?:\s*,\s*\d+)*\s*\]`)
		match := re.FindString(content)
		if match != "" {
			err = json.Unmarshal([]byte(match), &matchedIDs)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to parse AI response to []int: %v (Raw: %s)", err, content)
		}
	}

	log.Printf("[Groq AI Debug] Parsed IDs: %v\n", matchedIDs)
	return matchedIDs, nil
}

// GenerateEmbedding - Gọi HuggingFace API để tạo Vector cho nội dung (chuẩn bị dữ liệu cho vector search pgvector)
func (s *AIService) GenerateEmbedding(text string) ([]float32, error) {
	// Sử dụng model của HuggingFace (all-MiniLM-L6-v2: 384 dimensions)
	url := "https://router.huggingface.co/hf-inference/models/sentence-transformers/all-MiniLM-L6-v2/pipeline/feature-extraction"

	payload := map[string]interface{}{
		"inputs": text,
	}
	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.huggingFaceToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.huggingFaceToken)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("huggingface api request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		// Tránh spam log, trả về lỗi khi API rate limit
		return nil, fmt.Errorf("huggingface api error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var vector []float32
	if err := json.NewDecoder(resp.Body).Decode(&vector); err != nil {
		return nil, fmt.Errorf("failed to parse embedding response: %v", err)
	}

	return vector, nil
}

// AnswerFAQ nhận một câu hỏI từ người dùng và sử dụng RAG để trả lời dựa trên context của rạp phim, lịch sử người dùng, bắp nước và khuyến mãi
func (s *AIService) AnswerFAQ(question string, moviesData string, userContext string, servicesData string) (string, error) {
	if s.groqAPIKey == "" {
		return "", fmt.Errorf("GROQ_API_KEY is not configured")
	}

	systemContext := `Bạn là CHUYÊN GIA TƯ VẤN PHIM & DỊCH VỤ (Movie & Service Concierge) của hệ thống rạp BookingShow.
Nhiệm vụ: Phân tích gu xem phim, đề xuất phim phù hợp và tư vấn các dịch vụ đi kèm (bắp nước, khuyến mãi).

--- DỮ LIỆU PHIM ĐANG CHIẾU ---
` + moviesData + `

--- DỮ LIỆU BẮP NƯỚC & KHUYẾN MÃI ---
` + servicesData + `

--- NGỮ CẢNH NGƯỜI DÙNG (DÙNG ĐỂ PHÂN TÍCH GU) ---
` + userContext + `

HƯỚNG DẪN TƯ VẤN CỦA "MOVIE CONCIERGE":
1. PHÂN TÍCH & KẾT NỐI: Nhận diện sở thích người dùng. Nếu họ đã xem phim hành động, hãy nhắc lại.
2. ĐỀ XUẤT CÁ NHÂN HÓA: Chỉ đề xuất phim trong danh sách ĐANG CHIẾU. Giải thích vì sao nó hợp gu họ.
3. GIA TĂNG TRẢI NGHIỆM (CROSS-SELL):
   - Khi tư vấn phim hoặc suất chiếu, hãy khéo léo giới thiệu các Combo Bắp Nước phù hợp (Ví dụ: "Xem phim hành động kịch tính mà có thêm Combo bắp ngọt 2 ngăn thì đúng bài bạn ạ!").
   - Nếu người dùng có vẻ băn khoăn về giá hoặc đang muốn đặt vé, hãy giới thiệu các MÃ GIẢM GIÁ đang hoạt động để khích lệ họ.
4. THÚC ĐẨY HÀNH ĐỘNG: Gợi ý xem sơ đồ ghế (dùng tool get_seat_map) để chọn chỗ đẹp.

QUY TẮC PHẢN HỒI:
- Trình bày thông tin bắp nước hoặc khuyến mãi dưới dạng danh sách gọn gàng nếu được hỏi.
- Luôn giữ thái độ phục vụ chuyên nghiệp, tinh tế, am hiểu.
- KHÔNG gọi tool nếu thông tin đã có sẵn trong dữ liệu trên.`

	messages := []Message{
		{Role: "system", Content: systemContext},
		{Role: "user", Content: question},
	}

	maxRetries := 5 // Tăng số lần thử để AI Agent có thể suy nghĩ sâu hơn
	for i := 0; i < maxRetries; i++ {
		reqBody := GroqRequest{
			Model:       "llama-3.3-70b-versatile",
			Messages:    messages,
			Temperature: 0.1, // Nhiệt độ thấp cho Agent ổn định
			Tools:       GetAvailableTools(),
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return "", err
		}

		req, err := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(jsonData))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+s.groqAPIKey)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 45 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return "", fmt.Errorf("groq api error %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var groqRes GroqResponse
		if err := json.NewDecoder(resp.Body).Decode(&groqRes); err != nil {
			resp.Body.Close()
			return "", err
		}
		resp.Body.Close()

		if len(groqRes.Choices) == 0 {
			break
		}

		aiMsg := groqRes.Choices[0].Message
		messages = append(messages, aiMsg)

		// Nếu AI trả về nội dung và KHÔNG có yêu cầu gọi tool nữa, đó là kết quả cuối cùng
		if aiMsg.Content != "" && len(aiMsg.ToolCalls) == 0 {
			return aiMsg.Content, nil
		}

		// Nếu AI yêu cầu gọi tool
		if len(aiMsg.ToolCalls) > 0 {
			for _, toolCall := range aiMsg.ToolCalls {
				log.Printf("[AI AGENT] Calling Tool: %s with Args: %s\n", toolCall.Function.Name, toolCall.Function.Arguments)
				toolResult, err := ExecuteTool(toolCall.Function.Name, toolCall.Function.Arguments)
				if err != nil {
					toolResult = "Lỗi hệ thống: " + err.Error()
				}
				
				messages = append(messages, Message{
					Role:       "tool",
					Content:    toolResult,
					ToolCallID: toolCall.ID,
				})
			}
			// Tiếp tục loop để AI nhận kết quả tool và trả lời người dùng
			continue
		}
		
		// Fallback nếu AI trả về Content rỗng và cũng ko gọi tool (hiếm gặp)
		if aiMsg.Content != "" {
			return aiMsg.Content, nil
		}
	}

	return "Tôi đã tìm kiếm nhưng hiện tại không thể lấy đủ thông tin. Vui lòng hỏi lại hoặc liên hệ hotline 1900-1234 để được hỗ trợ nhanh nhất.", nil
}
