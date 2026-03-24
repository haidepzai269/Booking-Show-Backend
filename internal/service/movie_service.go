package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/pgvector/pgvector-go"
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

// HomeMovieDTO - Data Transfer Object cho phim trên trang chủ (có thêm trường Rating)
type HomeMovieDTO struct {
	model.Movie
	Rating float64 `json:"rating"`
}

// HomeMoviesResponse - Cấu trúc response cho trang chủ
type HomeMoviesResponse struct {
	Featured    *HomeMovieDTO  `json:"featured"`
	Hot         []HomeMovieDTO `json:"hot"`
	BestSelling []HomeMovieDTO `json:"best_selling"`
	ComingSoon  []HomeMovieDTO `json:"coming_soon"`
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

	// 2. Cache miss → Query DB
	log.Println("[Cache MISS] movies:home - querying DB")

	// Lọc Phim Nổi Bật (Featured) - Lấy 1 phim
	var featuredMovie model.Movie
	if err := repository.DB.Preload("Genres").Where("is_active = ? AND is_featured = ?", true, true).
		Order("release_date DESC").First(&featuredMovie).Error; err != nil {
		// Fallback: Lấy phim mới nhất nếu không có phim nào được đánh dấu Featured
		repository.DB.Preload("Genres").Where("is_active = ?", true).
			Order("release_date DESC").First(&featuredMovie)
	}

	// Lấy Phim Hot (Ưu tiên is_hot = true)
	var hotMovies []model.Movie
	if err := repository.DB.Preload("Genres").Where("is_active = ? AND is_hot = ?", true, true).
		Order("release_date DESC").Limit(8).Find(&hotMovies).Error; err != nil || len(hotMovies) == 0 {
		// Fallback: Lấy các phim mới nhất
		repository.DB.Preload("Genres").Where("is_active = ?", true).
			Order("release_date DESC").Limit(8).Find(&hotMovies)
	}

	if len(hotMovies) == 0 {
		return &HomeMoviesResponse{}, nil
	}

	// Hot list hiển thị (không bao gồm featured ở một số giao diện, nhưng ở đây ta cứ lấy đủ)
	hot := hotMovies
	if len(hot) > 4 {
		hot = hot[:4]
	}

	// Lấy Phim Bán Chạy (Best Selling - Ưu tiên is_best_selling = true)
	var bestSelling []model.Movie
	if err := repository.DB.Preload("Genres").Where("is_active = ? AND is_best_selling = ?", true, true).
		Order("release_date DESC").Limit(4).Find(&bestSelling).Error; err != nil || len(bestSelling) == 0 {
		// Fallback: Lấy các phim có release_date gần đây
		repository.DB.Preload("Genres").Where("is_active = ?", true).
			Order("release_date ASC").Limit(4).Find(&bestSelling)
	}

	// Lấy Phim Sắp Chiếu (release_date > today)
	var comingSoon []model.Movie
	if err := repository.DB.Preload("Genres").Where("is_active = ? AND release_date > ?", true, time.Now()).
		Order("release_date ASC").Limit(6).Find(&comingSoon).Error; err != nil {
		log.Printf("Warning: Failed to fetch coming soon movies: %v", err)
	}

	// Convert sang DTO và lấy Rating
	extraSvc := &MovieExtraService{}
	getRating := func(m model.Movie) HomeMovieDTO {
		extra, _ := extraSvc.GetExtraInfo(m.ID)
		rating := 8.5 // fallback mặc định nếu TMDB/Cache xịt
		if extra != nil && extra.Rating > 0 {
			rating = extra.Rating
		}
		return HomeMovieDTO{
			Movie:  m,
			Rating: rating,
		}
	}

	var hotDTOs []HomeMovieDTO
	for _, m := range hot {
		hotDTOs = append(hotDTOs, getRating(m))
	}

	var bestSellingDTOs []HomeMovieDTO
	for _, m := range bestSelling {
		bestSellingDTOs = append(bestSellingDTOs, getRating(m))
	}

	var comingSoonDTOs []HomeMovieDTO
	for _, m := range comingSoon {
		comingSoonDTOs = append(comingSoonDTOs, getRating(m))
	}

	featuredDTO := getRating(featuredMovie)

	result := &HomeMoviesResponse{
		Featured:    &featuredDTO,
		Hot:         hotDTOs,
		BestSelling: bestSellingDTOs,
		ComingSoon:  comingSoonDTOs,
	}

	// 3. Lưu vào Redis (TTL 5 phút cho trang chủ)
	if redispkg.Client != nil {
		if data, err := json.Marshal(result); err == nil {
			redispkg.Client.Set(redispkg.Ctx, cacheKeyMoviesHome, data, 2*time.Hour)
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
	IsHot           bool   `json:"is_hot"`
	IsBestSelling   bool   `json:"is_best_selling"`
	IsFeatured      bool   `json:"is_featured"`
}

func (s *MovieService) CreateMovie(req CreateMovieReq) (*model.Movie, error) {
	movie := model.Movie{
		Title:           req.Title,
		Description:     req.Description,
		DurationMinutes: req.DurationMinutes,
		PosterURL:       req.PosterURL,
		TrailerURL:      req.TrailerURL,
		IsHot:           req.IsHot,
		IsBestSelling:   req.IsBestSelling,
		IsFeatured:      req.IsFeatured,
	}

	if len(req.GenreIDs) > 0 {
		var genres []model.Genre
		repository.DB.Where("id IN ?", req.GenreIDs).Find(&genres)
		movie.Genres = genres
	}

	aiSvc := NewAIService("", "")
	if vec, err := aiSvc.GenerateEmbedding(req.Title + ". " + req.Description); err == nil && len(vec) == 1024 {
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
	IsHot           *bool  `json:"is_hot"`
	IsBestSelling   *bool  `json:"is_best_selling"`
	IsFeatured      *bool  `json:"is_featured"`
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
	if req.IsHot != nil {
		movie.IsHot = *req.IsHot
	}
	if req.IsBestSelling != nil {
		movie.IsBestSelling = *req.IsBestSelling
	}
	if req.IsFeatured != nil {
		movie.IsFeatured = *req.IsFeatured
	}
	if req.ReleaseDate != "" {
		parsed, err := time.Parse("2006-01-02", req.ReleaseDate)
		if err == nil {
			movie.ReleaseDate = parsed
		}
	}

	// Update embedding if title or description changed
	if req.Title != "" || req.Description != "" {
		aiSvc := NewAIService("", "")
		if vec, err := aiSvc.GenerateEmbedding(movie.Title + ". " + movie.Description); err == nil && len(vec) == 1024 {
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
func (s *MovieService) ListAdminMovies(page, limit int, q string, onlyActive bool, filter string) (*ListAdminMoviesResult, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	// 1. Ap dung bo loc co ban
	db := repository.DB.Model(&model.Movie{})
	if onlyActive {
		db = db.Where("is_active = ?", true)
	}
	if q != "" {
		db = db.Where("title ILIKE ?", "%"+q+"%")
	}

	// 2. Ap dung bo loc dac biet CHO COUNT (chi nhung cai lam thay doi so luong dong)
	if filter == "coming_soon" {
		db = db.Where("release_date > ?", time.Now())
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// 3. Ap dung logic lay du lieu (Preload, Joins, Order)
	query := repository.DB.Preload("Genres").
		Where("id IN (?)", repository.DB.Model(&model.Movie{}).Select("id").Where(db.Statement.Context.Value("gorm:db"))) // Reuse filters

	// Re-apply filters to a fresh query to avoid side effects from Count
	query = repository.DB.Preload("Genres")
	if onlyActive {
		query = query.Where("movies.is_active = ?", true)
	}
	if q != "" {
		query = query.Where("movies.title ILIKE ?", "%"+q+"%")
	}
	if filter == "coming_soon" {
		query = query.Where("movies.release_date > ?", time.Now())
	}

	switch filter {
	case "hot":
		query = query.Order("movies.is_hot DESC, movies.release_date DESC")
	case "coming_soon":
		query = query.Order("movies.release_date ASC")
	case "best_selling":
		// Uu tien phim duoc danh dau is_best_selling, sau do den doanh so thuc te
		query = query.Joins("LEFT JOIN showtimes ON showtimes.movie_id = movies.id").
			Joins("LEFT JOIN orders ON orders.showtime_id = showtimes.id AND orders.status = 'COMPLETED'").
			Group("movies.id").
			Select("movies.*, COUNT(orders.id) as sales_count").
			Order("movies.is_best_selling DESC, sales_count DESC")
	default:
		query = query.Order("movies.created_at DESC")
	}

	var movies []model.Movie
	if err := query.Limit(limit).Offset(offset).Find(&movies).Error; err != nil {
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

// GetMoviesMeta - Lấy metadata của các phim đang chiếu để làm context cho AI (RAG)
func (s *MovieService) GetMoviesMeta() (string, error) {
	cacheKey := "movies:meta:rag"
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, cacheKey).Result(); err == nil && cached != "" {
			log.Printf("[RAG Meta] Cache Hit (len: %d)\n", len(cached))
			return cached, nil
		}
	}

	var movies []model.Movie
	if err := repository.DB.Preload("Genres").Where("is_active = ?", true).Limit(50).Find(&movies).Error; err != nil {
		log.Printf("[RAG Meta Error]: %v\n", err)
		return "", err
	}
	log.Printf("[RAG Meta] Fetched %d movies from DB\n", len(movies))

	var sb strings.Builder
	for _, m := range movies {
		genres := ""
		for _, g := range m.Genres {
			genres += g.Name + ", "
		}
		sb.WriteString(fmt.Sprintf("ID: %d | Tên: %s | Thể loại: %s | Mô tả: %.200s\n", m.ID, m.Title, genres, m.Description))
	}

	meta := sb.String()
	if redispkg.Client != nil {
		redispkg.Client.Set(redispkg.Ctx, cacheKey, meta, 30*time.Minute)
	}
	return meta, nil
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

	// Lọc theo thể loại (nếu có)
	if req.GenreID > 0 {
		db = db.Joins("JOIN movie_genres ON movie_genres.movie_id = movies.id").
			Where("movie_genres.genre_id = ?", req.GenreID)
	}

	var movies []model.Movie

	if req.Query != "" {
		// 1. TÌM KIẾM TỪ KHÓA (Keyword Search) - Bao quát cả Tên, Mô tả và Thể loại
		var keywordMovies []model.Movie
		repository.DB.Model(&model.Movie{}).
			Preload("Genres").
			Joins("LEFT JOIN movie_genres ON movie_genres.movie_id = movies.id").
			Joins("LEFT JOIN genres ON genres.id = movie_genres.genre_id").
			Where("movies.is_active = ? AND (movies.title ILIKE ? OR movies.description ILIKE ? OR genres.name ILIKE ? OR ? ILIKE '%' || genres.name || '%')", 
				true, "%"+req.Query+"%", "%"+req.Query+"%", "%"+req.Query+"%", req.Query).
			Group("movies.id").
			Limit(10).
			Find(&keywordMovies)

		// 2. TÌM KIẾM NGỮ NGHĨA (Vector Search - Voyage AI 1024-dim)
		aiSvc := NewAIService("", "")
		embedding, err := aiSvc.GenerateEmbedding(req.Query)
		
		var vectorMovies []model.Movie
		if err == nil && len(embedding) == 1024 {
			vec := pgvector.NewVector(embedding)
			// Sử dụng Raw SQL để pgvector hoạt động 100% chính xác
			err := repository.DB.Raw(`
				SELECT * FROM movies 
				WHERE is_active = true AND embedding IS NOT NULL 
				ORDER BY embedding <=> ? 
				LIMIT ?
			`, vec, req.Limit).Scan(&vectorMovies).Error
			
			if err != nil {
				log.Printf("[AI Vector Search Error] Query: '%s' - Error: %v\n", req.Query, err)
			} else {
				// Preload Genres cho kết quả vector
				for i := range vectorMovies {
					repository.DB.Model(&vectorMovies[i]).Association("Genres").Find(&vectorMovies[i].Genres)
				}
				log.Printf("[AI Vector Search] Query: '%s' -> Found %d movies\n", req.Query, len(vectorMovies))
			}
		} else {
			log.Printf("[AI Search Warning] Voyage Embedding failed: %v", err)
		}

		// 3. HYBRID MERGE: Kết hợp kết quả từ cả  source, loại bỏ trùng lặp
		seen := make(map[int]bool)
		var combined []model.Movie

		// Thêm kết quả từ Keyword Search trước (độ chính xác cao về tên)
		for _, m := range keywordMovies {
			if !seen[m.ID] {
				combined = append(combined, m)
				seen[m.ID] = true
			}
		}
		// Thêm kết quả từ Vector Search (ngữ nghĩa)
		for _, m := range vectorMovies {
			if !seen[m.ID] {
				combined = append(combined, m)
				seen[m.ID] = true
			}
		}
		movies = combined
	} else {
		// Nếu không có query, chỉ lọc theo thể loại và sắp xếp
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

		if err := db.Limit(req.Limit).Find(&movies).Error; err != nil {
			return nil, err
		}
	}

	// Lưu kết quả vào Redis (5 phút)
	if redispkg.Client != nil && len(movies) > 0 {
		if data, err := json.Marshal(movies); err == nil {
			redispkg.Client.Set(redispkg.Ctx, cacheKey, data, 5*time.Minute)
		}
	}

	return movies, nil
}

// GetNowShowingMovies - Lấy danh sách phim đang có suất chiếu (sắp xếp theo thời gian chiếu sớm nhất)
func (s *MovieService) GetNowShowingMovies() ([]model.Movie, error) {
	var movies []model.Movie

	// Subquery để lấy phim và thời gian chiếu sớm nhất của nó
	subQuery := repository.DB.Model(&model.Showtime{}).
		Select("movie_id, MIN(start_time) as earliest").
		Where("start_time > ? AND is_active = ?", time.Now(), true).
		Group("movie_id")

	err := repository.DB.Preload("Genres").
		Table("movies").
		Joins("JOIN (?) as st ON st.movie_id = movies.id", subQuery).
		Where("movies.is_active = ?", true).
		Order("st.earliest ASC").
		Limit(15). // Lấy 15 phim (để xoay vòng 5 lần, mỗi lần 3 phim)
		Find(&movies).Error

	return movies, err
}
