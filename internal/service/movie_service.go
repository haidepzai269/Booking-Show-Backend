package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
)

const (
	cacheKeyMovies     = "movies:all"
	cacheKeyMoviesHome = "movies:home"
	cacheTTL           = 10 * time.Minute
)

type MovieService struct{}

// ListMovies - Lấy danh sách tất cả phim, có Redis cache
func (s *MovieService) ListMovies() ([]model.Movie, error) {
	// 1. Thử lấy từ Redis cache
	if redispkg.Client != nil {
		cached, err := redispkg.Client.Get(redispkg.Ctx, cacheKeyMovies).Result()
		if err == nil {
			var movies []model.Movie
			if json.Unmarshal([]byte(cached), &movies) == nil {
				log.Println("[Cache HIT] movies:all")
				return movies, nil
			}
		}
	}

	// 2. Cache miss → Lấy từ DB
	log.Println("[Cache MISS] movies:all - querying DB")
	var movies []model.Movie
	if err := repository.DB.Preload("Genres").Where("is_active = ?", true).
		Order("release_date DESC").Find(&movies).Error; err != nil {
		return nil, err
	}

	// 3. Lưu vào Redis cache
	if redispkg.Client != nil {
		if data, err := json.Marshal(movies); err == nil {
			redispkg.Client.Set(redispkg.Ctx, cacheKeyMovies, data, cacheTTL)
		}
	}

	return movies, nil
}

// HomeMoviesResponse - Cấu trúc response cho trang chủ
type HomeMoviesResponse struct {
	Featured    *model.Movie  `json:"featured"`
	Hot         []model.Movie `json:"hot"`
	BestSelling []model.Movie `json:"best_selling"`
}

// GetHomeMovies - Lấy dữ liệu phim cho trang chủ (featured, hot, best-selling)
// Có Redis cache riêng để tối ưu performance
func (s *MovieService) GetHomeMovies() (*HomeMoviesResponse, error) {
	// 1. Thử lấy từ Redis cache
	if redispkg.Client != nil {
		cached, err := redispkg.Client.Get(redispkg.Ctx, cacheKeyMoviesHome).Result()
		if err == nil {
			var result HomeMoviesResponse
			if json.Unmarshal([]byte(cached), &result) == nil {
				log.Println("[Cache HIT] movies:home")
				return &result, nil
			}
		}
	}

	// 2. Cache miss → Query DB
	log.Println("[Cache MISS] movies:home - querying DB")

	// Lấy 8 phim mới nhất làm "hot"
	var hotMovies []model.Movie
	if err := repository.DB.Preload("Genres").Where("is_active = ?", true).
		Order("release_date DESC").Limit(8).Find(&hotMovies).Error; err != nil {
		return nil, err
	}

	if len(hotMovies) == 0 {
		return &HomeMoviesResponse{}, nil
	}

	// Featured = phim đầu tiên (mới nhất)
	featured := hotMovies[0]
	// Hot = 4 phim đầu (không bao gồm featured)
	hot := hotMovies
	if len(hot) > 4 {
		hot = hot[:4]
	}

	// Best selling = lấy theo thứ tự doanh số (từ orders/tickets - đơn giản hóa: lấy 4 phim tiếp theo)
	var bestSelling []model.Movie
	if err := repository.DB.Preload("Genres").Where("is_active = ?", true).
		Order("release_date ASC").Limit(4).Find(&bestSelling).Error; err != nil {
		return nil, err
	}

	result := &HomeMoviesResponse{
		Featured:    &featured,
		Hot:         hot,
		BestSelling: bestSelling,
	}

	// 3. Lưu vào Redis (TTL 5 phút cho trang chủ)
	if redispkg.Client != nil {
		if data, err := json.Marshal(result); err == nil {
			redispkg.Client.Set(redispkg.Ctx, cacheKeyMoviesHome, data, 5*time.Minute)
		}
	}

	return result, nil
}

func (s *MovieService) GetMovie(id int) (*model.Movie, error) {
	cacheKey := fmt.Sprintf("movies:%d", id)

	// Thử lấy từ Redis cache
	if redispkg.Client != nil {
		cached, err := redispkg.Client.Get(redispkg.Ctx, cacheKey).Result()
		if err == nil {
			var movie model.Movie
			if json.Unmarshal([]byte(cached), &movie) == nil {
				log.Printf("[Cache HIT] movie:%d\n", id)
				return &movie, nil
			}
		}
	}

	var movie model.Movie
	if err := repository.DB.Preload("Genres").Where("is_active = ?", true).First(&movie, id).Error; err != nil {
		return nil, errors.New("movie not found")
	}

	// Cache movie chi tiết trong 15 phút
	if redispkg.Client != nil {
		if data, err := json.Marshal(movie); err == nil {
			redispkg.Client.Set(redispkg.Ctx, cacheKey, data, 15*time.Minute)
		}
	}

	return &movie, nil
}

type CreateMovieReq struct {
	Title           string `json:"title" binding:"required"`
	Description     string `json:"description"`
	DurationMinutes int    `json:"duration_minutes" binding:"required,gt=0"`
	ReleaseDate     string `json:"release_date" binding:"required"` // Format: YYYY-MM-DD
	PosterURL       string `json:"poster_url"`
	TrailerURL      string `json:"trailer_url"`
	GenreIDs        []int  `json:"genre_ids"`
}

func (s *MovieService) CreateMovie(req CreateMovieReq) (*model.Movie, error) {
	movie := model.Movie{
		Title:           req.Title,
		Description:     req.Description,
		DurationMinutes: req.DurationMinutes,
		PosterURL:       req.PosterURL,
		TrailerURL:      req.TrailerURL,
	}

	if len(req.GenreIDs) > 0 {
		var genres []model.Genre
		repository.DB.Where("id IN ?", req.GenreIDs).Find(&genres)
		movie.Genres = genres
	}

	// Auto generate embedding from Title and Description
	aiSvc := NewAIService("")
	if vec, err := aiSvc.GenerateEmbedding(req.Title + ". " + req.Description); err == nil && len(vec) == 384 {
		v := pgvector.NewVector(vec)
		movie.Embedding = &v
	} else {
		log.Printf("Warning: Failed to generate embedding for new movie: %v", err)
	}

	if err := repository.DB.Create(&movie).Error; err != nil {
		return nil, err
	}

	// Invalidate cache khi tạo phim mới
	if redispkg.Client != nil {
		redispkg.Client.Del(redispkg.Ctx, cacheKeyMovies, cacheKeyMoviesHome)
	}

	return &movie, nil
}

func (s *MovieService) DeleteMovie(id int) error {
	result := repository.DB.Model(&model.Movie{}).Where("id = ?", id).Update("is_active", false)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("movie not found")
	}

	// Invalidate cache
	if redispkg.Client != nil {
		cacheKey := fmt.Sprintf("movies:%d", id)
		redispkg.Client.Del(redispkg.Ctx, cacheKeyMovies, cacheKeyMoviesHome, cacheKey)
	}

	return nil
}

// UpdateMovieReq - request body cho cập nhật phim
type UpdateMovieReq struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	DurationMinutes int    `json:"duration_minutes"`
	ReleaseDate     string `json:"release_date"` // Format: YYYY-MM-DD
	PosterURL       string `json:"poster_url"`
	TrailerURL      string `json:"trailer_url"`
	GenreIDs        []int  `json:"genre_ids"`
	IsActive        *bool  `json:"is_active"`
}

// UpdateMovie - Cập nhật thông tin phim
func (s *MovieService) UpdateMovie(id int, req UpdateMovieReq) (*model.Movie, error) {
	var movie model.Movie
	if err := repository.DB.Preload("Genres").First(&movie, id).Error; err != nil {
		return nil, errors.New("movie not found")
	}

	// Cập nhật các fields không rỗng
	if req.Title != "" {
		movie.Title = req.Title
	}
	if req.Description != "" {
		movie.Description = req.Description
	}
	if req.DurationMinutes > 0 {
		movie.DurationMinutes = req.DurationMinutes
	}
	if req.PosterURL != "" {
		movie.PosterURL = req.PosterURL
	}
	if req.TrailerURL != "" {
		movie.TrailerURL = req.TrailerURL
	}
	if req.IsActive != nil {
		movie.IsActive = *req.IsActive
	}
	if req.ReleaseDate != "" {
		parsed, err := time.Parse("2006-01-02", req.ReleaseDate)
		if err == nil {
			movie.ReleaseDate = parsed
		}
	}

	// Update embedding if title or description changed
	if req.Title != "" || req.Description != "" {
		aiSvc := NewAIService("")
		if vec, err := aiSvc.GenerateEmbedding(movie.Title + ". " + movie.Description); err == nil && len(vec) == 384 {
			v := pgvector.NewVector(vec)
			movie.Embedding = &v
		}
	}

	if err := repository.DB.Save(&movie).Error; err != nil {
		return nil, err
	}

	// Cập nhật genres nếu có
	if req.GenreIDs != nil {
		var genres []model.Genre
		if len(req.GenreIDs) > 0 {
			repository.DB.Where("id IN ?", req.GenreIDs).Find(&genres)
		}
		if err := repository.DB.Model(&movie).Association("Genres").Replace(genres); err != nil {
			return nil, err
		}
		movie.Genres = genres
	}

	// Invalidate các cache liên quan
	if redispkg.Client != nil {
		cacheKey := fmt.Sprintf("movies:%d", id)
		redispkg.Client.Del(redispkg.Ctx, cacheKeyMovies, cacheKeyMoviesHome, cacheKey)
		// Xóa search cache
		iter := redispkg.Client.Scan(redispkg.Ctx, 0, "movies:search:*", 0).Iterator()
		var keys []string
		for iter.Next(redispkg.Ctx) {
			keys = append(keys, iter.Val())
		}
		if len(keys) > 0 {
			redispkg.Client.Del(redispkg.Ctx, keys...)
		}
	}

	return &movie, nil
}

// ListAdminMoviesResult - kết quả có pagination
type ListAdminMoviesResult struct {
	Movies []model.Movie `json:"movies"`
	Total  int64         `json:"total"`
	Page   int           `json:"page"`
	Limit  int           `json:"limit"`
}

// ListAdminMovies - danh sách phim cho admin (có pagination, filter, search)
func (s *MovieService) ListAdminMovies(page, limit int, q string, onlyActive bool) (*ListAdminMoviesResult, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	db := repository.DB.Preload("Genres")
	if onlyActive {
		db = db.Where("is_active = ?", true)
	}
	if q != "" {
		db = db.Where("title ILIKE ?", "%"+q+"%")
	}

	var total int64
	if err := db.Model(&model.Movie{}).Count(&total).Error; err != nil {
		return nil, err
	}

	var movies []model.Movie
	if err := db.Order("created_at DESC").Limit(limit).Offset(offset).Find(&movies).Error; err != nil {
		return nil, err
	}

	return &ListAdminMoviesResult{
		Movies: movies,
		Total:  total,
		Page:   page,
		Limit:  limit,
	}, nil
}

type GenreService struct{}

func (s *GenreService) ListGenres() ([]model.Genre, error) {
	var genres []model.Genre
	if err := repository.DB.Find(&genres).Error; err != nil {
		return nil, err
	}
	return genres, nil
}

// SearchMoviesReq - tham số tìm kiếm
type SearchMoviesReq struct {
	Query   string // tìm theo tên phim
	GenreID int    // lọc theo thể loại (0 = tất cả)
	Sort    string // "release_date", "title" — mặc định release_date DESC
	Limit   int    // mặc định 20
}

// SearchMovies - Tim phim theo tu khoa ket hop RAG AI (Groq) & Fuzzy Search (pg_trgm)
func (s *MovieService) SearchMovies(req SearchMoviesReq) ([]model.Movie, error) {
	if req.Limit <= 0 || req.Limit > 50 {
		req.Limit = 20
	}

	// 0. Redis Cache Check (TTL 5 phut)
	cacheKey := fmt.Sprintf("movies:search:%s:%d:%s:%d", req.Query, req.GenreID, req.Sort, req.Limit)
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, cacheKey).Result(); err == nil {
			var movies []model.Movie
			if json.Unmarshal([]byte(cached), &movies) == nil {
				log.Printf("[Cache HIT] %s\n", cacheKey)
				return movies, nil
			}
		}
	}
	log.Printf("[Cache MISS] %s - querying AI/DB\n", cacheKey)

	db := repository.DB.Preload("Genres").Where("is_active = ?", true)

	// Loc theo the loai
	if req.GenreID > 0 {
		db = db.Joins("JOIN movie_genres ON movie_genres.movie_id = movies.id").
			Where("movie_genres.genre_id = ?", req.GenreID)
	}

	var movies []model.Movie

	if req.Query != "" {
		// 1. Thu dung AI Search (RAG) truoc neu query co y nghia (du dai)
		if len(req.Query) >= 3 {
			aiSvc := NewAIService("")
			embedding, err := aiSvc.GenerateEmbedding(req.Query)

			if err == nil && len(embedding) > 0 {
				vec := pgvector.NewVector(embedding)
				// Dùng Cosine Distance (<=>) của pgvector
				aiDB := repository.DB.Preload("Genres").Where("is_active = ?", true).
					Order(gorm.Expr("NULLIF(embedding::text, '')::vector <=> ?", vec))

				if err := aiDB.Limit(req.Limit).Find(&movies).Error; err != nil {
					return nil, err
				}

				if len(movies) > 0 {
					log.Printf("[AI Vector Search] Query: '%s' -> Found %d\n", req.Query, len(movies))
					if redispkg.Client != nil {
						if data, err := json.Marshal(movies); err == nil {
							redispkg.Client.Set(redispkg.Ctx, cacheKey, data, 5*time.Minute)
						}
					}
					return movies, nil
				}
			} else {
				log.Printf("[AI Vector Error]: %v\n", err)
			}
		}

		// 2. Fallback: pg_trgm Fuzzy Search
		log.Printf("[Fuzzy DB Search] Query: '%s'\n", req.Query)
		db = db.Where("title ILIKE ? OR similarity(title, ?) > 0.15", "%"+req.Query+"%", req.Query).
			Order(gorm.Expr("similarity(title, ?) DESC", req.Query))
	} else {
		switch req.Sort {
		case "title":
			db = db.Order("title ASC")
		case "title_desc":
			db = db.Order("title DESC")
		case "oldest":
			db = db.Order("release_date ASC")
		default:
			db = db.Order("release_date DESC")
		}
	}

	if err := db.Limit(req.Limit).Find(&movies).Error; err != nil {
		return nil, err
	}

	// Luu ket qua vao Redis (5 phut)
	if redispkg.Client != nil {
		if data, err := json.Marshal(movies); err == nil {
			redispkg.Client.Set(redispkg.Ctx, cacheKey, data, 5*time.Minute)
		}
	}

	return movies, nil
}
