package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/booking-show/booking-show-api/internal/service"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/gin-gonic/gin"
)

type FAQHandler struct{}

func NewFAQHandler() *FAQHandler {
	return &FAQHandler{}
}

func (h *FAQHandler) AskFAQ(c *gin.Context) {
	var req struct {
		Question string `json:"question" binding:"required"`
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
	var allMovies []model.Movie
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, "movies:all").Result(); err == nil {
			_ = json.Unmarshal([]byte(cached), &allMovies)
		}
	}
	if len(allMovies) == 0 {
		repository.DB.Preload("Genres").Where("is_active = ?", true).Find(&allMovies)
	}

	var metaStr strings.Builder
	var wg sync.WaitGroup
	var mu sync.Mutex
	extraSvc := &service.MovieExtraService{}

	for i, m := range allMovies {
		wg.Add(1)
		go func(index int, movie model.Movie) {
			defer wg.Done()
			genreNames := ""
			for _, g := range movie.Genres {
				genreNames += g.Name + " "
			}

			// Gọi service extra để lấy đạo diễn & diễn viên qua Redis/TMDB
			// Quá trình này được chạy song song cho tất cả các phim nên cực kỳ nhanh
			extra, err := extraSvc.GetExtraInfo(movie.ID)
			director := "Đang cập nhật"
			castStr := "Đang cập nhật"

			if err == nil && extra != nil {
				director = extra.Director
				var casts []string
				for _, c := range extra.Cast {
					casts = append(casts, c.Name)
				}
				if len(casts) > 0 {
					castStr = strings.Join(casts, ", ")
				}
			}

			info := fmt.Sprintf("%d. %s - Phân loại: %s. Thời lượng: %d phút.\nĐạo diễn: %s. Diễn viên chính: %s.\nTóm tắt: %.100s\n",
				index+1, movie.Title, genreNames, movie.DurationMinutes, director, castStr, movie.Description)

			// Lock để viết vào chuỗi RAG an toàn
			mu.Lock()
			metaStr.WriteString(info)
			mu.Unlock()
		}(i, m)
	}
	wg.Wait()

	aiSvc := service.NewAIService("", "")
	answer, err := aiSvc.AnswerFAQ(req.Question, metaStr.String())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "AI đang bận, xin vui lòng thử lại sau.", "details": err.Error()})
		return
	}

	// LƯU CÂU HỎI VÀ CÂU TRẢ LỜI VÀO DATABASE DƯỚI BACKGROUND
	go func(q, a string) {
		var faq model.FAQLog
		if err := repository.DB.Where("question ILIKE ?", q).First(&faq).Error; err == nil {
			faq.AskCount++
			faq.Answer = a // Cập nhật câu trả lời mới nhất
			repository.DB.Save(&faq)
		} else {
			repository.DB.Create(&model.FAQLog{Question: q, Answer: a, AskCount: 1})
		}

		// Xóa cache cũ để hệ thống cập nhật lại câu hỏi Top ngay khi người dùng F5 web
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
