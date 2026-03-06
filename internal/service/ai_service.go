package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type AIService struct {
	groqAPIKey string
}

func NewAIService(apiKey string) *AIService {
	return &AIService{
		groqAPIKey: apiKey,
	}
}

// GroqRequest đại diện cho body request gửi lên Groq
type GroqRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float32   `json:"temperature"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GroqResponse đại diện cho dữ liệu trả về
type GroqResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
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

	client := &http.Client{Timeout: 5 * time.Second} // Groq rất nhanh, 5s là đủ
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

	var matchedIDs []int
	err = json.Unmarshal([]byte(content), &matchedIDs)
	if err != nil {
		// Thử lọc nếu AI có chen thêm text không mong muốn (mặc dù đã cấm bằng system prompt)
		return nil, fmt.Errorf("failed to parse AI response to []int: %v (Raw: %s)", err, content)
	}

	return matchedIDs, nil
}

// GenerateEmbedding - Gọi HuggingFace API để tạo Vector cho nội dung (chuẩn bị dữ liệu cho vector search pgvector)
func (s *AIService) GenerateEmbedding(text string) ([]float32, error) {
	// Sử dụng model miễn phí của HuggingFace (all-MiniLM-L6-v2: 384 dimensions)
	// Lưu ý: Trong môi trường production thực tế, nên cung cấp Authorization: Bearer <HF_TOKEN>
	// hoặc tự host pipeline python. Trong project này, ta gọi thẳng public inference.
	url := "https://api-inference.huggingface.co/pipeline/feature-extraction/sentence-transformers/all-MiniLM-L6-v2"

	payload := map[string]interface{}{
		"inputs": text,
	}
	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

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

// AnswerFAQ nhận một câu hỏi từ người dùng và sử dụng RAG để trả lời dựa trên context của rạp phim
func (s *AIService) AnswerFAQ(question string, moviesData string) (string, error) {
	if s.groqAPIKey == "" {
		return "", fmt.Errorf("GROQ_API_KEY is not configured")
	}

	systemContext := `Bạn là trợ lý ảo AI thông minh và lịch sự của hệ thống đặt vé xem phim BookingShow.
Chỉ trả lời câu hỏi dựa trên các thông tin được cung cấp dưới đây. Nếu câu hỏi nằm ngoài phạm vi hoạt động của rạp chiếu phim BookingShow hoặc bạn không biết câu trả lời, hãy tế nhị từ chối và hướng dẫn khách hàng liên hệ hotline 1900-1234. Trả lời bằng tiếng Việt, ngắn gọn, súc tích và thân thiện. TRÌNH BÀY DƯỚI DẠNG TEXT ĐƠN GIẢN HOẶC MARKDOWN CƠ BẢN (KHÔNG DÙNG CÁC THẺ HTML).

--- THÔNG TIN RẠP PHIM BOOKINGSHOW ---
1. Về BookingShow: Là hệ thống rạp chiếu phim hiện đại hàng đầu có mặt tại các thành phố lớn ở Việt Nam (Hà Nội, TP.HCM, Đà Nẵng). Chúng tôi mang lại trải nghiệm xem phim đỉnh cao với công nghệ IMAX, 4DX và âm thanh Dolby Atmos.
2. Mua vé & Thanh toán: Chúng tôi hỗ trợ mua vé trực tuyến qua website. Chấp nhận thanh toán bằng Thẻ tín dụng, ZaloPay, PayOS và quét mã QR. 
3. Quy định Hủy / Đổi vé: Rất tiếc, BookingShow hiện chưa hỗ trợ hoàn hủy vé hoặc đổi suất chiếu sau khi đã thanh toán thành công để đảm bảo quyền lợi cho các khách hàng khác. Vui lòng kiểm tra lại thông tin trước khi thanh toán.
4. Giá vé (Pricing):
   - Vé Tiêu Chuẩn (Standard): 80,000 VND - 100,000 VND (tùy vị trí ghế và rạp).
   - Vé VIP (Ghế đôi/Sweetbox): 150,000 VND - 200,000 VND.
   - Bắp nước (Concessions): Combo 1 bắp 1 nước từ 65,000 VND. Combo 1 bắp 2 nước từ 85,000 VND. Đặt kèm vé sẽ rẻ hơn 10% so với mua tại quầy.
5. Kiểm soát vé: Mua vé trực tuyến, bạn sẽ nhận được mã QR CODE. Chỉ cần đưa QR CODE trên điện thoại cho nhân viên quét ở cửa rạp là có thể vào phòng chiếu, không cần xếp hàng lấy vé giấy cứng.
6. Chương trình thành viên / Tích điểm: Sắp ra mắt.
7. Liên hệ: Hotline hỗ trợ: 1900-1234, Email: support@bookingshow.vn.

--- DANH SÁCH CÁC BỘ PHIM HIỆN ĐANG ĐƯỢC CHIẾU TẠI RẠP ---
Nếu người dùng hỏi có bao nhiêu bộ phim, hãy đếm số lượng dựa trên danh sách dưới đây và có thể điểm tên một vài phim nổi bật để gợi ý (Thể loại, độ dài). Nếu danh sách dưới đây trống, hãy thông báo hiện rạp đang cập nhật danh sách phim mới.
` + moviesData

	reqBody := GroqRequest{
		Model: "llama-3.3-70b-versatile",
		Messages: []Message{
			{Role: "system", Content: systemContext},
			{Role: "user", Content: question},
		},
		Temperature: 0.3, // RAG thường để temperature thấp
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create http request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.groqAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call Groq API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("groq api error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var groqRes GroqResponse
	if err := json.NewDecoder(resp.Body).Decode(&groqRes); err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}

	if len(groqRes.Choices) == 0 {
		return "Tôi đang gặp sự cố kết nối, xin vui lòng thử lại sau.", nil
	}

	return groqRes.Choices[0].Message.Content, nil
}
