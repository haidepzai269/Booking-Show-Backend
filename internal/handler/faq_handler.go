package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/booking-show/booking-show-api/internal/service"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/gin-gonic/gin"
	"github.com/pgvector/pgvector-go"
)

type FAQHandler struct{}

func NewFAQHandler() *FAQHandler {
	return &FAQHandler{}
}

func (h *FAQHandler) AskFAQ(c *gin.Context) {
	var req struct {
		Question  string `json:"question" binding:"required"`
		SessionID string `json:"session_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Vui lòng nhập câu hỏi hợp lệ"})
		return
	}

	req.Question = strings.TrimSpace(req.Question)

	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Chưa trỏ cấu hình AI RAG. Vui lòng liên hệ Admin/Quản trị viên."})
		return
	}

	// Xóa đoạn goroutine save db cũ ở đây vì chưa có answer

	// Bốc dữ liệu phim đang chiếu
	aiSvc := service.NewAIService("", "")

	// 1. Tìm ngữ cảnh liên quan bằng Vector Search (Voyage AI 1024-dim)
	knowledgeContext, err := aiSvc.SearchContext(req.Question)
	if err != nil {
		log.Printf("Warning: Vector Search context failed: %v", err)
		knowledgeContext = "Hiện tại không lấy được thông tin chi tiết từ cơ sở tri thức."
	}

	// LẤY NGỮ CẢNH NGƯỜI DÙNG (LỊCH SỬ XEM PHIM)
	userContext := "Người dùng chưa có lịch sử mua vé hoặc chưa đăng nhập."
	if val, exists := c.Get("userID"); exists {
		uid := val.(int)
		// Lấy lịch sử đơn hàng của user để AI biết user đã mua gì
		orderSvc := &service.OrderService{}
		orders, _, err := orderSvc.MyOrders(uid, 1, 5) // Lấy 5 đơn gần nhất
		if err == nil {
			var histStr strings.Builder
			histStr.WriteString("Lịch sử xem phim của người dùng: ")
			seenMovies := make(map[string]bool)
			for _, o := range orders {
				if o.Showtime.Movie.Title != "" && !seenMovies[o.Showtime.Movie.Title] {
					seenMovies[o.Showtime.Movie.Title] = true
					genreNames := ""
					for _, g := range o.Showtime.Movie.Genres {
						genreNames += g.Name + " "
					}
					histStr.WriteString(fmt.Sprintf("- %s (%s). ", o.Showtime.Movie.Title, strings.TrimSpace(genreNames)))
				}
			}
			userContext = histStr.String()
		}
	}

	// CHUẨN HÓA CÂU HỎI ĐỂ CHECK CACHE (Semantic Level 1)
	normalizedQ := strings.ToLower(req.Question)
	normalizedQ = strings.TrimSuffix(normalizedQ, "?")
	normalizedQ = strings.TrimSpace(normalizedQ)

	cacheKey := fmt.Sprintf("faq:answer:%s", normalizedQ)
	var answer string

	// 1. Kiểm tra Redis Cache trước
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, cacheKey).Result(); err == nil && cached != "" {
			answer = cached
		}
	}

	// 2. Nếu Redis rỗng, kiểm tra DB (FAQLog)
	if answer == "" {
		var faq model.FAQLog
		// Tìm câu hỏi tương tự (ILIKE) và đã có câu trả lời
		if err := repository.DB.Where("question ILIKE ? AND answer IS NOT NULL AND answer != ''", req.Question).First(&faq).Error; err == nil {
			answer = faq.Answer
			// Lưu ngược lại vào Redis để lần sau nhanh hơn
			if redispkg.Client != nil {
				_ = redispkg.Client.Set(redispkg.Ctx, cacheKey, answer, 24*time.Hour).Err()
			}
		}
	}

	// 3. Nếu vẫn không có mới gọi AI
	if answer == "" {
		var err error
		answer, err = aiSvc.AnswerFAQ(req.Question, knowledgeContext, userContext)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "AI đang bận, xin vui lòng thử lại sau.", "details": err.Error()})
			return
		}
		// Lưu vào Redis cho những lần sau
		if redispkg.Client != nil {
			_ = redispkg.Client.Set(redispkg.Ctx, cacheKey, answer, 24*time.Hour).Err()
		}
	}

	// Lấy userIDPtr từ context (nếu có)
	var userIDPtr *uint
	if val, exists := c.Get("userID"); exists {
		uid := val.(int)
		uidUint := uint(uid)
		userIDPtr = &uidUint
	}

	// CHỈ LƯU TIN NHẮN VÀO LỊCH SỬ CHAT NẾU LÀ NGƯỜI DÙNG ĐÃ ĐĂNG NHẬP
	if userIDPtr != nil {
		chatSvc := service.NewChatService()
		_ = chatSvc.SaveMessage(userIDPtr, req.SessionID, "user", req.Question)
		_ = chatSvc.SaveMessage(userIDPtr, req.SessionID, "ai", answer)
	}

	// LƯU CÂU HỎI VÀ CÂU TRẢ LỜI VÀO DATABASE FAQ (THỐNG KÊ THÔNG MINH)
	go func(q, a string) {
		// Chỉ lưu các câu hỏi đủ dài để đảm bảo chất lượng FAQ
		if len(strings.TrimSpace(q)) < 10 {
			return
		}

		aiSvc := service.NewAIService("", "")
		vec, err := aiSvc.GenerateEmbedding("Câu hỏi thường gặp/FAQ: " + q)
		if err != nil || len(vec) != 1024 {
			return
		}
		v := pgvector.NewVector(vec)

		var faq model.FAQLog
		// 1. Tìm câu hỏi tương đồng nhất bằng Vector (ngưỡng 0.1 tương đương ~90% khớp ý nghĩa)
		err = repository.DB.Raw(`
			SELECT * FROM faq_logs 
			WHERE embedding IS NOT NULL 
			ORDER BY embedding <=> ? 
			LIMIT 1
		`, v).Scan(&faq).Error

		// Đo khoảng cách vector (nếu DB hỗ trợ hàm <=> trong SELECT)
		// Ở đây đơn giản: Nếu tìm thấy 1 câu gần nhất và khoảng cách đủ nhỏ (ta ưu tiên exact match trước cho nhanh)
		
		isSimilar := false
		if err == nil && faq.ID != 0 {
			// Kiểm tra khoảng cách thực tế (Cosine Distance)
			var distance float64
			repository.DB.Raw("SELECT ? <=> ? as dist", v, faq.Embedding).Scan(&distance)
			if distance < 0.1 {
				isSimilar = true
			}
		}

		if isSimilar {
			// Tăng điểm cho câu hỏi cũ nếu cùng ý nghĩa
			faq.AskCount++
			faq.Answer = a // Cập nhật câu trả lời mới nhất của AI
			faq.UpdatedAt = time.Now()
			repository.DB.Save(&faq)
		} else {
			// Tạo mới nếu nội dung khác biệt hoàn toàn
			newFaq := model.FAQLog{
				Question:  q,
				Answer:    a,
				AskCount:  1,
				Embedding: &v,
			}
			repository.DB.Create(&newFaq)
		}

		// Xóa cache cũ để hệ thống cập nhật lại câu hỏi Top
		if redispkg.Client != nil {
			redispkg.Client.Del(redispkg.Ctx, "faq:top3")
		}
	}(req.Question, answer)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"answer": answer,
		},
	})
}

// FAQResponse struct để trả về JSON
type FAQResponse struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// GetTopFAQs trả về danh sách các câu hỏi phổ biến nhất
func (h *FAQHandler) GetTopFAQs(c *gin.Context) {
	cacheKey := "faq:top3"

	// 1. Thử lấy từ Redis Cache
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, cacheKey).Result(); err == nil && cached != "" {
			var result []FAQResponse
			if json.Unmarshal([]byte(cached), &result) == nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
				return
			}
		}
	}

	// 2. Nếu Cache rỗng, truy xuất CSDL
	var faqs []model.FAQLog
	if err := repository.DB.Order("ask_count DESC").Limit(10).Find(&faqs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Lỗi truy xuất cơ sở dữ liệu."})
		return
	}

	var results []FAQResponse

	if len(faqs) == 0 {
		// Fallback nếu DB trống
		results = []FAQResponse{
			{Question: "Giá vé xem phim là bao nhiêu tiền?", Answer: "Chào bạn. Hiện tại hệ thống BookingShow có 2 mức giá: Vé ghế Standard là 80.000 VNĐ. Vé ghế đôi VIP Sweetbox là 150.000 VNĐ (được tặng kèm voucher để mua bắp nước)."},
			{Question: "Tôi có thể đổi/hủy suất chiếu sau khi thanh toán không?", Answer: "Rất tiếc, theo quy định của hệ thống Bookingshow, chúng tôi chưa hỗ trợ hoàn hủy vé đã thanh toán thành công."},
			{Question: "Rạp có bán bắp nước online không?", Answer: "Tất nhiên rồi! Khi đặt vé trên website, hệ thống sẽ đề xuất Combo Bắp Ngọt, Nước Có Ga và Snack để bạn đặt và thanh toán trước với giá siêu rẻ."},
		}
	} else {
		seen := make(map[string]bool)
		for _, f := range faqs {
			cleanQ := strings.ToLower(strings.TrimSpace(f.Question))
			if !seen[cleanQ] {
				seen[cleanQ] = true
				results = append(results, FAQResponse{
					Question: strings.TrimSpace(f.Question),
					Answer:   f.Answer,
				})
				if len(results) == 3 {
					break // Chỉ lấy đủ 3 câu top
				}
			}
		}
	}

	// 3. Save ngược lại vào Server với thời gian sống 1 tiếng
	if redispkg.Client != nil {
		if dataBytes, err := json.Marshal(results); err == nil {
			_ = redispkg.Client.Set(redispkg.Ctx, cacheKey, dataBytes, 1*time.Hour).Err()
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
	})
}
