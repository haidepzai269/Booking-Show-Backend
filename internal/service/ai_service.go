package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/pgvector/pgvector-go"
)

type AIService struct {
	groqAPIKey   string
	voyageAPIKey string
}

func NewAIService(groqKey, voyageKey string) *AIService {
	// Nếu truyền vào trống, thử đọc từ env (để hỗ trợ các chỗ gọi chưa refactor)
	if groqKey == "" {
		groqKey = os.Getenv("GROQ_API_KEY")
	}
	if voyageKey == "" {
		voyageKey = os.Getenv("VOYAGE_API_KEY")
	}

	return &AIService{
		groqAPIKey:   groqKey,
		voyageAPIKey: voyageKey,
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

// GenerateEmbedding - Gọi Voyage AI API để tạo Vector cho nội dung (chuẩn bị dữ liệu cho vector search pgvector)
func (s *AIService) GenerateEmbedding(text string) ([]float32, error) {
	// 1. Kiểm tra Redis Cache trước để tránh gọi API lặp lại (tiết kiệm RPM/Token)
	h := sha256.New()
	h.Write([]byte(text))
	textHash := hex.EncodeToString(h.Sum(nil))
	redisKey := "embedding:cache:v3:" + textHash

	if redis.Client != nil {
		cached, err := redis.Client.Get(redis.Ctx, redisKey).Result()
		if err == nil {
			var cachedVec []float32
			if err := json.Unmarshal([]byte(cached), &cachedVec); err == nil {
				log.Printf("[Embedding Cache HIT] text: %.30s...\n", text)
				return cachedVec, nil
			}
		}
	}

	// 2. Nếu Cache MISS -> Gọi Voyage AI API (voyage-3)
	url := "https://api.voyageai.com/v1/embeddings"
	payload := map[string]interface{}{
		"input": text,
		"model": "voyage-3",
	}
	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.voyageAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.voyageAPIKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyage ai api request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("voyage ai api error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse embedding response: %v", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned from voyage ai")
	}

	embedding := result.Data[0].Embedding

	// 3. Lưu vào Redis Cache (TTL 7 ngày - vì Vector của cùng 1 text là cố định)
	if redis.Client != nil {
		if data, err := json.Marshal(embedding); err == nil {
			redis.Client.Set(redis.Ctx, redisKey, string(data), 7*24*time.Hour)
			log.Printf("[Embedding Cache SET] text: %.30s...\n", text)
		}
	}

	return embedding, nil
}

// AnswerFAQ nhận một câu hỏi từ người dùng và sử dụng RAG để trả lời dựa trên context đã được tìm kiếm
func (s *AIService) AnswerFAQ(question string, knowledgeContext string, userContext string) (string, error) {
	if s.groqAPIKey == "" {
		return "", fmt.Errorf("GROQ_API_KEY is not configured")
	}

	systemContext := `Bạn là CHUYÊN GIA TƯ VẤN PHIM & DỊCH VỤ (Movie & Service Concierge) của hệ thống rạp BookingShow.
Nhiệm vụ: Phân tích gu xem phim, đề xuất phim phù hợp và tư vấn các dịch vụ đi kèm (bắp nước, khuyến mãi).

--- DỮ LIỆU TRI THỨC (KNOWLEDGE BASE) ---
` + knowledgeContext + `

--- NGỮ CẢNH NGƯỜI DÙNG (DÙNG ĐỂ PHÂN TÍCH GU) ---
` + userContext + `

HƯỚNG DẪN TƯ VẤN CỦA "MOVIE CONCIERGE":
1. PHÂN TÍCH & KẾT NỐI: Nhận diện sở thích người dùng. Nếu họ đã xem phim hành động, hãy nhắc lại.
2. ĐỀ XUẤT CÁ NHÂN HÓA: Chỉ đề xuất phim hoặc dịch vụ có trong DANH SÁCH TRI THỨC trên. Giải thích vì sao nốt hợp gu họ.
3. GIA TĂNG TRẢI NGHIỆM (CROSS-SELL):
   - Khi tư vấn phim, hãy khéo léo giới thiệu các Combo Bắp Nước hoặc Khuyến mãi phù hợp đang có trong tri thức.
4. THÚC ĐẨY HÀNH ĐỘNG: Gợi ý xem sơ đồ ghế (get_seat_map) hoặc đặt vé ngay.

QUY TẮC PHẢN HỒI:
- Trình bày thông tin rõ ràng, chuyên nghiệp.
- Không bịa đặt thông tin không có trong tri thức.`

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

// SearchContext - Tìm kiếm ngữ cảnh liên quan từ nhiều nguồn (Movies, Concessions, Promotions, FAQs)
func (s *AIService) SearchContext(query string) (string, error) {
	// 1. Kiểm tra Redis Cache cho Context (TTL 1 giờ)
	// Việc cache cả chuỗi context giúp ChatBot phản hồi cực nhanh cho các câu hỏi phổ biến
	h := sha256.New()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(query))))
	queryHash := hex.EncodeToString(h.Sum(nil))
	redisKey := "chatbot:context:v1:" + queryHash

	if redis.Client != nil {
		cached, err := redis.Client.Get(redis.Ctx, redisKey).Result()
		if err == nil && cached != "" {
			log.Printf("[ChatBot Context Cache HIT] query: %.30s...\n", query)
			return cached, nil
		}
	}

	// 2. Nếu Cache MISS -> Tiến hành tìm kiếm (đã có cache embedding bên trong GenerateEmbedding)
	embedding, err := s.GenerateEmbedding(query)
	if err != nil {
		return "", err
	}
	vec := pgvector.NewVector(embedding)

	var sb strings.Builder
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	// 0. Nếu query liên quan đến "suất chiếu" hoặc "lịch chiếu", ưu tiên lấy phim Now Showing
	lowerQuery := strings.ToLower(query)
	if strings.Contains(lowerQuery, "suất chiếu") || strings.Contains(lowerQuery, "đang chiếu") || strings.Contains(lowerQuery, "lịch chiếu") {
		movieSvc := &MovieService{}
		nowShowing, _ := movieSvc.GetNowShowingMovies()
		if len(nowShowing) > 0 {
			sb.WriteString("\n--- PHIM ĐANG CÓ SUẤT CHIẾU TẠI RẠP ---\n")
			for _, m := range nowShowing {
				sb.WriteString(fmt.Sprintf("- %s (ID: %d)\n", m.Title, m.ID))
			}
		}
	}

	// 1. Tìm Phim liên quan (Hybrid Search: Keyword + Vector)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var keywordMovies []model.Movie
		var vectorMovies []model.Movie

		// 1a. Keyword Search quét Title, Description và THỂ LOẠI (Cực kỳ quan trọng cho câu hỏi "hoạt hình")
		repository.DB.Model(&model.Movie{}).
			Preload("Genres").
			Joins("LEFT JOIN movie_genres ON movie_genres.movie_id = movies.id").
			Joins("LEFT JOIN genres ON genres.id = movie_genres.genre_id").
			Where("movies.is_active = ? AND (movies.title ILIKE ? OR movies.description ILIKE ? OR genres.name ILIKE ? OR ? ILIKE '%' || genres.name || '%')", 
				true, "%"+query+"%", "%"+query+"%", "%"+query+"%", query).
			Group("movies.id").
			Limit(5).
			Find(&keywordMovies)

		// 1b. Vector Search tìm ý nghĩa tiềm ẩn
		repository.DB.Raw(`
			SELECT * FROM movies 
			WHERE is_active = true AND embedding IS NOT NULL 
			ORDER BY embedding <=> ? 
			LIMIT 5
		`, vec).Scan(&vectorMovies)

		// Trộn kết quả và loại trùng
		seen := make(map[int]bool)
		var merged []model.Movie
		
		// Ưu tiên Keyword trước (vì nó chính xác tuyệt đối với thể loại)
		for _, m := range keywordMovies {
			if !seen[m.ID] {
				merged = append(merged, m)
				seen[m.ID] = true
			}
		}
		// Sau đó bồi thêm Vector
		for _, m := range vectorMovies {
			if !seen[m.ID] {
				// Preload Genres thủ công cho kết quả Vector
				idCopy := m.ID
				repository.DB.Model(&model.Movie{ID: idCopy}).Association("Genres").Find(&m.Genres)
				merged = append(merged, m)
				seen[m.ID] = true
			}
		}

		if len(merged) > 0 {
			mu.Lock()
			sb.WriteString("\n--- DANH SÁCH PHIM PHÙ HỢP ---\n")
			for _, m := range merged {
				genres := ""
				for _, g := range m.Genres {
					genres += g.Name + " "
				}
				sb.WriteString(fmt.Sprintf("- %s (ID: %d). Thể loại: %s. Mô tả: %s\n", 
					m.Title, m.ID, strings.TrimSpace(genres), m.Description))
			}
			mu.Unlock()
		}
	}()

	// 2. Tìm Bắp nước liên quan
	wg.Add(1)
	go func() {
		defer wg.Done()
		var concessions []model.Concession
		repository.DB.Raw(`
			SELECT * FROM concessions 
			WHERE is_active = true AND embedding IS NOT NULL 
			ORDER BY embedding <=> ? 
			LIMIT 3
		`, vec).Scan(&concessions)
		
		if len(concessions) > 0 {
			mu.Lock()
			sb.WriteString("\n--- BẮP NƯỚC & ĐỒ ĂN ---\n")
			for _, c := range concessions {
				sb.WriteString(fmt.Sprintf("- %s: %d VNĐ. %s\n", c.Name, c.Price, c.Description))
			}
			mu.Unlock()
		}
	}()

	// 3. Tìm Khuyến mãi liên quan
	wg.Add(1)
	go func() {
		defer wg.Done()
		var promos []model.Promotion
		now := time.Now()
		repository.DB.Raw(`
			SELECT * FROM promotions 
			WHERE is_active = true AND embedding IS NOT NULL 
			AND (valid_from <= $1 AND valid_until >= $2)
			ORDER BY embedding <=> $3 
			LIMIT 2
		`, now, now, vec).Scan(&promos)
		
		if len(promos) > 0 {
			mu.Lock()
			sb.WriteString("\n--- KHUYẾN MÃI & VOUCHER ---\n")
			for _, p := range promos {
				sb.WriteString(fmt.Sprintf("- Mã %s: Giảm %d VNĐ. %s\n", p.Code, p.DiscountAmount, p.Description))
			}
			mu.Unlock()
		}
	}()

	// 4. Tìm FAQ liên quan (Tri thức cũ)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var faqs []model.FAQLog
		repository.DB.Raw(`
			SELECT * FROM faq_logs 
			WHERE answer IS NOT NULL AND answer != '' AND embedding IS NOT NULL
			ORDER BY embedding <=> ? 
			LIMIT 2
		`, vec).Scan(&faqs)
		
		if len(faqs) > 0 {
			mu.Lock()
			sb.WriteString("\n--- CÂU HỎI TƯƠNG TỰ ---\n")
			for _, f := range faqs {
				sb.WriteString(fmt.Sprintf("Q: %s\nA: %s\n", f.Question, f.Answer))
			}
			mu.Unlock()
		}
	}()

	wg.Wait()
	result := sb.String()

	// 3. Lưu vào Redis Cache (TTL 1 giờ - đảm bảo dữ liệu mới nhất nếu DB thay đổi)
	if redis.Client != nil && result != "" {
		redis.Client.Set(redis.Ctx, redisKey, result, 1*time.Hour)
		log.Printf("[ChatBot Context Cache SET] query: %.30s...\n", query)
	}

	return result, nil
}

// SeatLayoutReq dùng để gửi dữ liệu tối giản cho AI
type SeatLayoutID struct {
	ID         int    `json:"id"`
	RowChar    string `json:"row"`
	SeatNumber int    `json:"num"`
}

// DesignSeatLayout nhận yêu cầu của admin và danh sách ghế hiện tại, trả về danh sách ghế đã được gán tọa độ X, Y, Angle
func (s *AIService) DesignSeatLayout(prompt string, currentSeats []model.Seat) ([]model.Seat, error) {
	if s.groqAPIKey == "" {
		return nil, fmt.Errorf("GROQ_API_KEY is not configured")
	}

	// 1. Kiểm tra Redis Cache
	// Hash dựa trên prompt + số lượng ghế để đảm bảo tính duy nhất
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s:%d:v2", prompt, len(currentSeats))))
	promptHash := hex.EncodeToString(h.Sum(nil))
	redisKey := "seat_layout:ai:v2:" + promptHash

	if redis.Client != nil {
		cached, err := redis.Client.Get(redis.Ctx, redisKey).Result()
		if err == nil {
			var result []model.Seat
			if err := json.Unmarshal([]byte(cached), &result); err == nil {
				log.Printf("[AI Designer V2] Redis Cache Hit for: %s\n", prompt)
				return result, nil
			}
		}
	}

	// 2. Chuẩn bị dữ liệu gửi cho AI (tối giản để tiết kiệm token)
	tinySeats := make([]SeatLayoutID, len(currentSeats))
	for i, seat := range currentSeats {
		tinySeats[i] = SeatLayoutID{
			ID:         seat.ID,
			RowChar:    seat.RowChar,
			SeatNumber: seat.SeatNumber,
		}
	}
	seatsJSON, _ := json.Marshal(tinySeats)

	systemPrompt := `Bạn là Kiến trúc sư Rạp chiếu phim AI (Cinema Architect) cấp cao.
Nhiệm vụ của bạn là sắp xếp tọa độ (x, y) và góc xoay (angle) cho các ghế trong phòng chiếu dựa trên yêu cầu của Admin.

QUY TẮC KỸ THUẬT:
- KHÔNG GIAN: ViewBox rạp là 1000x800. Tâm rạp (500, 400). Màn hình ở phía trên (y=0).
- TỌA ĐỘ: x [0-1000], y [0-800].
- KHOẢNG CÁCH GHẾ: Khoảng cách tiêu chuẩn giữa 2 ghế cạnh nhau là 50 đơn vị. Khoảng cách giữa 2 hàng là 60 đơn vị.
- LỐI ĐI (AISLE): Nếu yêu cầu "chia bên", "chia cụm", "có lối đi ở giữa", hãy tạo một khoảng trống lớn (ít nhất 150 đơn vị) ở giữa các cụm ghế bằng cách điều chỉnh tọa độ X.
- CĂN CHỈNH: Luôn cố gắng căn giữa toàn bộ khối ghế theo trục X=500 để sơ đồ cân đối.

VÍ DỤ BỐ CỤC:
1. "Chia 2 bên": Chia danh sách ghế thành 2 nửa (theo SeatNumber). Nửa trái có x < 425, nửa phải có x > 575.
2. "Hình chữ nhật": Xếp các ghế cùng RowChar trên cùng một đường thẳng Y, giãn cách X đều nhau.
3. "Hình thoi/Vòm": Điều chỉnh Y tăng/giảm dần theo vị trí ghế trong hàng để tạo độ cong/vát.

KẾT QUẢ: Chỉ trả về JSON mảng: [{"id": int, "x": float, "y": float, "angle": float}]. KHÔNG GIẢI THÍCH.

DỮ LIỆU GHẾ HIỆN CÓ:
` + string(seatsJSON)

	userPrompt := "Yêu cầu thiết kế: " + prompt

	reqBody := GroqRequest{
		Model: "llama-3.3-70b-versatile",
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.1, // Giảm xuống 0.1 để cực kỳ chính xác hình học
	}

	jsonData, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.groqAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("groq api error %d", resp.StatusCode)
	}

	var groqRes GroqResponse
	json.NewDecoder(resp.Body).Decode(&groqRes)

	if len(groqRes.Choices) == 0 {
		return nil, fmt.Errorf("AI không trả về kết quả")
	}

	content := groqRes.Choices[0].Message.Content
	
	// Trích xuất JSON mảng từ content (phòng trường hợp AI trả về text dư thừa)
	re := regexp.MustCompile(`\[\s*\{.*\}\s*\]`)
	match := re.FindString(content)
	if match == "" {
		// Thử tìm [ ... ] đơn giản nhất
		reSimple := regexp.MustCompile(`\[[\s\S]*\]`)
		match = reSimple.FindString(content)
	}

	if match == "" {
		return nil, fmt.Errorf("không tìm thấy JSON hợp lệ trong phản hồi của AI: %s", content)
	}

	var aiResults []struct {
		ID    int     `json:"id"`
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
		Angle float64 `json:"angle"`
	}
	if err := json.Unmarshal([]byte(match), &aiResults); err != nil {
		return nil, fmt.Errorf("lỗi parse JSON AI: %v", err)
	}

	// 3. Map kết quả AI trở lại danh sách ghế gốc
	resultSeats := make([]model.Seat, len(currentSeats))
	copy(resultSeats, currentSeats)

	for i := range resultSeats {
		for _, ai := range aiResults {
			if resultSeats[i].ID == ai.ID {
				resultSeats[i].X = ai.X
				resultSeats[i].Y = ai.Y
				resultSeats[i].Angle = ai.Angle
				break
			}
		}
	}

	// 4. Lưu vào Redis Cache (TTL 24h)
	if redis.Client != nil {
		jsonData, _ := json.Marshal(resultSeats)
		redis.Client.Set(redis.Ctx, redisKey, string(jsonData), 24*time.Hour)
	}

	return resultSeats, nil
}
