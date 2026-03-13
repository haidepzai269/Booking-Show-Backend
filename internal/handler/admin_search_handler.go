package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/booking-show/booking-show-api/internal/service"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
)

// TTL constants
const (
	adminSearchTTL   = 3 * time.Minute  // Cache toàn bộ kết quả search
	adminAISearchTTL = 10 * time.Minute // Cache riêng kết quả AI (ổn định hơn)
)

type AdminSearchHandler struct{}

func NewAdminSearchHandler() *AdminSearchHandler {
	return &AdminSearchHandler{}
}

type SearchResultMovie struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Poster   string `json:"poster"`
	IsActive bool   `json:"is_active"`
	AIMatch  bool   `json:"ai_match,omitempty"`
}

type SearchResultUser struct {
	ID       int    `json:"id"`
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

type SearchResultOrder struct {
	ID          string `json:"id"`
	UserName    string `json:"user_name"`
	MovieTitle  string `json:"movie_title"`
	FinalAmount int    `json:"final_amount"`
	Status      string `json:"status"`
}

type AdminSearchResponse struct {
	Movies []SearchResultMovie `json:"movies"`
	Users  []SearchResultUser  `json:"users"`
	Orders []SearchResultOrder `json:"orders"`
	Query  string              `json:"query"`
	AIUsed bool                `json:"ai_used"`
}

// Search — GET /api/v1/admin/search?q=...
// Tối ưu: Redis multi-layer cache + Goroutine parallel queries + AI cache riêng
func (h *AdminSearchHandler) Search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "query phải có ít nhất 2 ký tự"})
		return
	}

	// =========================================================
	// LAYER 1: Cache toàn bộ kết quả (TTL 3 phút)
	// Nếu cùng query đã được tìm gần đây → trả về ngay
	// =========================================================
	fullCacheKey := fmt.Sprintf("admin:search:%s", strings.ToLower(q))
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, fullCacheKey).Result(); err == nil {
			var result AdminSearchResponse
			if json.Unmarshal([]byte(cached), &result) == nil {
				log.Printf("[Admin Search Cache HIT] q='%s'", q)
				c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
				return
			}
		}
	}

	like := "%" + q + "%"
	aiUsed := false

	// =========================================================
	// LAYER 2: Parallel goroutines cho tất cả queries
	// Movies (AI + fuzzy), Users, Orders chạy đồng thời
	// =========================================================
	var (
		movieResults []SearchResultMovie
		userResults  []SearchResultUser
		orderResults []SearchResultOrder
		wg           sync.WaitGroup
		mu           sync.Mutex
	)

	// --- Goroutine 1: Tìm phim (AI + fuzzy) ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		movies := searchMoviesWithAI(q, like, &aiUsed)
		mu.Lock()
		movieResults = movies
		mu.Unlock()
	}()

	// --- Goroutine 2: Tìm user ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		users := searchUsers(like)
		mu.Lock()
		userResults = users
		mu.Unlock()
	}()

	// --- Goroutine 3: Tìm đơn hàng ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		orders := searchOrders(like)
		mu.Lock()
		orderResults = orders
		mu.Unlock()
	}()

	wg.Wait()

	// Đảm bảo không nil
	if movieResults == nil {
		movieResults = []SearchResultMovie{}
	}
	if userResults == nil {
		userResults = []SearchResultUser{}
	}
	if orderResults == nil {
		orderResults = []SearchResultOrder{}
	}

	result := AdminSearchResponse{
		Movies: movieResults,
		Users:  userResults,
		Orders: orderResults,
		Query:  q,
		AIUsed: aiUsed,
	}

	// =========================================================
	// LAYER 3: Lưu toàn bộ kết quả vào cache (TTL 3 phút)
	// =========================================================
	if redispkg.Client != nil {
		if data, err := json.Marshal(result); err == nil {
			redispkg.Client.Set(redispkg.Ctx, fullCacheKey, data, adminSearchTTL)
			log.Printf("[Admin Search Cache SET] q='%s' TTL=%v ai=%v", q, adminSearchTTL, aiUsed)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// searchMoviesWithAI: Tìm phim ưu tiên AI Groq, fallback fuzzy/ILIKE
// Cache AI result riêng với TTL 10 phút
func searchMoviesWithAI(q, like string, aiUsed *bool) []SearchResultMovie {
	// --- Kiểm tra cache AI riêng ---
	aiCacheKey := fmt.Sprintf("admin:search:ai:%s", strings.ToLower(q))
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, aiCacheKey).Result(); err == nil {
			var movies []SearchResultMovie
			if json.Unmarshal([]byte(cached), &movies) == nil && len(movies) > 0 {
				log.Printf("[Admin AI Search Cache HIT] q='%s'", q)
				*aiUsed = true
				return movies
			}
		}
	}

	// --- Thử AI LLM Phân tích ngữ nghĩa nếu query đủ dài (Hybrid LLM Search) ---
	if len(q) >= 3 {
		movieSvc := &service.MovieService{}
		meta, _ := movieSvc.GetMoviesMeta()
		if meta != "" {
			aiSvc := service.NewAIService("", "")
			matchedIDs, err := aiSvc.AnalyzeSearchQuery(q, meta)
			if err == nil && len(matchedIDs) > 0 {
				log.Printf("[Admin AI LLM Search] q='%s' -> Matched IDs: %v", q, matchedIDs)
				var allMovies []model.Movie
				repository.DB.Where("id IN ? AND is_active = ?", matchedIDs, true).Find(&allMovies)

				if len(allMovies) > 0 {
					// Sắp xếp theo thứ tự AI trả về
					idMap := make(map[int]model.Movie)
					for _, m := range allMovies {
						idMap[m.ID] = m
					}
					var results []SearchResultMovie
					for _, id := range matchedIDs {
						if m, ok := idMap[id]; ok {
							results = append(results, SearchResultMovie{
								ID:       m.ID,
								Title:    m.Title,
								Poster:   m.PosterURL,
								IsActive: m.IsActive,
								AIMatch:  true,
							})
						}
					}

					*aiUsed = true
					if redispkg.Client != nil {
						if data, err := json.Marshal(results); err == nil {
							redispkg.Client.Set(redispkg.Ctx, aiCacheKey, data, adminAISearchTTL)
						}
					}
					return results
				}
			}
		}

		// --- Fallback: AI Vector Search ---
		aiSvc := service.NewAIService("", "")
		embedding, err := aiSvc.GenerateEmbedding(q)
		if err == nil && len(embedding) > 0 {
			var allMovies []model.Movie
			vec := pgvector.NewVector(embedding)

			repository.DB.Where("is_active = ?", true).
				Order(gorm.Expr("NULLIF(embedding::text, '')::vector <=> ?", vec)).
				Limit(5).
				Find(&allMovies)

			log.Printf("[Admin AI Vector Search] q='%s' -> Found: %v", q, len(allMovies))

			var results []SearchResultMovie
			for _, m := range allMovies {
				results = append(results, SearchResultMovie{
					ID:       m.ID,
					Title:    m.Title,
					Poster:   m.PosterURL,
					IsActive: m.IsActive,
					AIMatch:  true,
				})
			}

			if len(results) > 0 {
				*aiUsed = true
				// Cache kết quả AI riêng (TTL 10 phút)
				if redispkg.Client != nil {
					if data, err := json.Marshal(results); err == nil {
						redispkg.Client.Set(redispkg.Ctx, aiCacheKey, data, adminAISearchTTL)
					}
				}
				return results
			}
		} else {
			log.Printf("[Admin AI Vector Search Error] q='%s': %v", q, err)
		}
	}

	// --- Fallback: Fuzzy / ILIKE ---
	log.Printf("[Admin Fuzzy Search] q='%s'", q)
	var movies []model.Movie
	repository.DB.
		Where("title ILIKE ?", like).
		Select("id, title, poster_url, is_active").
		Limit(5).
		Find(&movies)

	results := make([]SearchResultMovie, 0, len(movies))
	for _, m := range movies {
		results = append(results, SearchResultMovie{
			ID:       m.ID,
			Title:    m.Title,
			Poster:   m.PosterURL,
			IsActive: m.IsActive,
		})
	}
	return results
}

// searchUsers: Tìm user theo full_name hoặc email
func searchUsers(like string) []SearchResultUser {
	var users []model.User
	repository.DB.
		Where("full_name ILIKE ? OR email ILIKE ?", like, like).
		Select("id, full_name, email, role").
		Limit(5).
		Find(&users)

	results := make([]SearchResultUser, 0, len(users))
	for _, u := range users {
		results = append(results, SearchResultUser{
			ID:       u.ID,
			FullName: u.FullName,
			Email:    u.Email,
			Role:     string(u.Role),
		})
	}
	return results
}

// searchOrders: Tìm đơn hàng theo user name, email, movie title, hoặc order ID
func searchOrders(like string) []SearchResultOrder {
	type rawOrder struct {
		ID          string
		UserName    string
		MovieTitle  string
		FinalAmount int
		Status      string
	}
	var orders []rawOrder

	q := strings.Trim(like, "%")
	isUUID := false
	if _, err := uuid.Parse(q); err == nil {
		isUUID = true
	}

	query := repository.DB.Table("orders").
		Select("orders.id, users.full_name as user_name, movies.title as movie_title, orders.final_amount, orders.status").
		Joins("JOIN users ON users.id = orders.user_id").
		Joins("JOIN showtimes ON showtimes.id = orders.showtime_id").
		Joins("JOIN movies ON movies.id = showtimes.movie_id")

	if isUUID {
		query = query.Where("orders.id = ?", q)
	} else {
		query = query.Where("users.full_name ILIKE ? OR users.email ILIKE ? OR movies.title ILIKE ?", like, like, like)
	}

	query.Limit(5).Scan(&orders)

	results := make([]SearchResultOrder, 0, len(orders))
	for _, o := range orders {
		results = append(results, SearchResultOrder{
			ID:          o.ID,
			UserName:    o.UserName,
			MovieTitle:  o.MovieTitle,
			FinalAmount: o.FinalAmount,
			Status:      o.Status,
		})
	}
	return results
}
