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
)

type ReviewService struct {
	BlacklistedSvc *BlacklistedWordService
}

func NewReviewService() *ReviewService {
	return &ReviewService{
		BlacklistedSvc: &BlacklistedWordService{},
	}
}

func (s *ReviewService) invalidateMovieReviewCache(movieID int) {
	if redispkg.Client != nil {
		// Xóa cache theo movie_id cho tất cả các trang/sort
		keysPattern := fmt.Sprintf("movies:%d:reviews:*", movieID)
		iter := redispkg.Client.Scan(redispkg.Ctx, 0, keysPattern, 0).Iterator()
		var keys []string
		for iter.Next(redispkg.Ctx) {
			keys = append(keys, iter.Val())
		}
		if len(keys) > 0 {
			redispkg.Client.Del(redispkg.Ctx, keys...)
		}
	}
}

func (s *ReviewService) invalidateAdminReviewCache() {
	if redispkg.Client != nil {
		// Xóa tất cả cache liên quan đến admin reviews
		keysPattern := "admin:reviews:*"
		iter := redispkg.Client.Scan(redispkg.Ctx, 0, keysPattern, 0).Iterator()
		var keys []string
		for iter.Next(redispkg.Ctx) {
			keys = append(keys, iter.Val())
		}
		if len(keys) > 0 {
			redispkg.Client.Del(redispkg.Ctx, keys...)
		}
	}
}

type RatingStats struct {
	TotalReviews  int64            `json:"total_reviews"`
	AverageRating float64          `json:"average_rating"`
	RatingCounts  map[int]int      `json:"rating_counts"`
}

// GetMovieReviews - Lấy danh sách reviews kèm stats
func (s *ReviewService) GetMovieReviews(movieID int, sort string, ratingFilter int, page, limit int, currentUserID int) ([]model.Review, *RatingStats, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	offset := (page - 1) * limit

	// Cache key kết hợp lấy luôn trạng thái liked nếu là user
	cacheKey := fmt.Sprintf("movies:%d:reviews:sort=%s:rating=%d:page=%d:limit=%d:uid=%d", movieID, sort, ratingFilter, page, limit, currentUserID)
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, cacheKey).Result(); err == nil {
			var result struct {
				Reviews []model.Review `json:"reviews"`
				Stats   *RatingStats   `json:"stats"`
			}
			if json.Unmarshal([]byte(cached), &result) == nil {
				return result.Reviews, result.Stats, nil
			}
		}
	}

	db := repository.DB.Where("movie_id = ? AND status = ?", movieID, model.StatusPublished).Preload("User")

	var total int64
	if err := db.Model(&model.Review{}).Count(&total).Error; err != nil {
		return nil, nil, err
	}

	// Calculate Stats
	type Result struct {
		Rating int
		Count  int
	}
	var results []Result
	repository.DB.Model(&model.Review{}).Where("movie_id = ? AND status = ?", movieID, model.StatusPublished).
		Select("rating, count(*) as count").Group("rating").Scan(&results)

	stats := &RatingStats{
		TotalReviews:  total,
		AverageRating: 0,
		RatingCounts:  map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0},
	}
	
	var totalStars int
	for _, r := range results {
		stats.RatingCounts[r.Rating] = r.Count
		totalStars += r.Rating * r.Count
	}
	
	if total > 0 {
		stats.AverageRating = float64(totalStars) / float64(total)
	}

	switch sort {
	case "popular":
		db = db.Order("likes_count DESC, created_at DESC")
	case "oldest":
		db = db.Order("created_at ASC")
	default:
		db = db.Order("created_at DESC") // newest
	}

	if ratingFilter > 0 {
		db = db.Where("rating = ?", ratingFilter)
	}

	var reviews []model.Review
	if err := db.Limit(limit).Offset(offset).Find(&reviews).Error; err != nil {
		return nil, nil, err
	}

	// Nếu user đăng nhập, kiểm tra liked
	if currentUserID > 0 && len(reviews) > 0 {
		var reviewIDs []int
		for _, r := range reviews {
			reviewIDs = append(reviewIDs, r.ID)
		}
		var userLikes []model.ReviewLike
		repository.DB.Where("user_id = ? AND review_id IN ?", currentUserID, reviewIDs).Find(&userLikes)
		
		likedMap := make(map[int]bool)
		for _, l := range userLikes {
			likedMap[l.ReviewID] = true
		}
		// Sẽ không lưu thuộc tính liked trực tiếp vào model DB được vì GORM model không có
		// ta xử lý tạm bằng API handler. Nhưng ở đây trả Raw Reviews
	}

	// Save to Cache (5 mins)
	if redispkg.Client != nil {
		dataReq := struct {
			Reviews []model.Review `json:"reviews"`
			Stats   *RatingStats   `json:"stats"`
		}{reviews, stats}
		if data, err := json.Marshal(dataReq); err == nil {
			redispkg.Client.Set(redispkg.Ctx, cacheKey, data, 5*time.Minute)
		}
	}

	return reviews, stats, nil
}

func (s *ReviewService) CreateReview(userID, movieID, rating int, content string) (*model.Review, error) {
	// 1. Kiểm tra User Lock state
	var user model.User
	if err := repository.DB.First(&user, userID).Error; err != nil {
		return nil, errors.New("user not found")
	}

	if user.StrikeCount >= 10 || (user.MutedUntil != nil && user.MutedUntil.After(time.Now())) {
		return nil, errors.New("Bạn đã bị khóa quyền bình luận do vi phạm nhiều lần")
	}

	// 2. Kiểm tra xem người dùng đã mua vé phim này chưa
	var order model.Order
	err := repository.DB.Joins("JOIN showtimes ON showtimes.id = orders.showtime_id").
		Where("orders.user_id = ? AND showtimes.movie_id = ? AND orders.status = ?", userID, movieID, model.OrderCompleted).
		First(&order).Error
	if err != nil {
		return nil, errors.New("Bạn cần mua vé (và đã thanh toán thành công) để có thể đánh giá phim này")
	}

	// 3. Lớp 1: Kiểm duyệt bằng Danh sách đen (Blacklisted Word)
	isToxic, err := s.BlacklistedSvc.CheckIfContainsBlacklistedWord(content)
	if err != nil {
		return nil, err
	}

	if isToxic {
		// Tăng strike_count
		user.StrikeCount += 1
		if user.StrikeCount >= 10 {
			muteTime := time.Now().Add(7 * 24 * time.Hour) // Khóa 7 ngày
			user.MutedUntil = &muteTime
		}
		repository.DB.Save(&user)
		return nil, errors.New("Nội dung của bạn chứa từ ngữ không phù hợp và đã bị chặn. Vui lòng giữ gìn sự văn minh.")
	}

	// 4. Lưu Db
	review := model.Review{
		MovieID: movieID,
		UserID:  userID,
		Rating:  rating,
		Content: content,
		Status:  model.StatusPublished,
	}

	if err := repository.DB.Create(&review).Error; err != nil {
		return nil, err
	}

	// 5. Đẩy vào Redis Queue cho Lớp 2 (AI Moderation)
	if redispkg.Client != nil {
		jobData, _ := json.Marshal(map[string]interface{}{
			"review_id": review.ID,
			"content":   content,
			"user_id":   userID,
		})
		redispkg.Client.LPush(redispkg.Ctx, "queue:ai_moderation", jobData)
	} else {
		log.Println("Redis is nil, cannot push to ai moderation queue")
	}

	s.invalidateMovieReviewCache(movieID)
	s.invalidateAdminReviewCache()

	return &review, nil
}

func (s *ReviewService) DeleteReview(userID, reviewID int, isAdmin bool) error {
	var review model.Review
	if err := repository.DB.First(&review, reviewID).Error; err != nil {
		return errors.New("review not found")
	}

	if !isAdmin && review.UserID != userID {
		return errors.New("unauthorized to delete this review")
	}

	if err := repository.DB.Delete(&review).Error; err != nil {
		return err
	}

	s.invalidateMovieReviewCache(review.MovieID)
	s.invalidateAdminReviewCache()

	if redispkg.Client != nil {
		channel := fmt.Sprintf("movie:%d:reviews", review.MovieID)
		payload, _ := json.Marshal(map[string]interface{}{
			"action":    "DELETED",
			"review_id": reviewID,
		})
		redispkg.Client.Publish(redispkg.Ctx, channel, string(payload))
	}

	return nil
}

func (s *ReviewService) ToggleLikeReview(userID, reviewID int) error {
	var review model.Review
	if err := repository.DB.First(&review, reviewID).Error; err != nil {
		return errors.New("review not found")
	}

	// Thử tìm like
	var like model.ReviewLike
	err := repository.DB.Where("user_id = ? AND review_id = ?", userID, reviewID).First(&like).Error
	if err == nil {
		// Bỏ like
		repository.DB.Delete(&like)
		repository.DB.Model(&review).UpdateColumn("likes_count", review.LikesCount-1)
	} else {
		// Thêm like
		newLike := model.ReviewLike{UserID: userID, ReviewID: reviewID}
		repository.DB.Create(&newLike)
		repository.DB.Model(&review).UpdateColumn("likes_count", review.LikesCount+1)
	}

	s.invalidateMovieReviewCache(review.MovieID)
	s.invalidateAdminReviewCache()
	return nil
}

// CÁC HÀM CHO ADMIN
func (s *ReviewService) ListAdminReviews(page, limit int, status string) ([]model.Review, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	offset := (page - 1) * limit

	// Redis Cache cho Admin (30 giây)
	cacheKey := fmt.Sprintf("admin:reviews:status=%s:page=%d:limit=%d", status, page, limit)
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, cacheKey).Result(); err == nil {
			var result struct {
				Reviews []model.Review `json:"reviews"`
				Total   int64          `json:"total"`
			}
			if json.Unmarshal([]byte(cached), &result) == nil {
				return result.Reviews, result.Total, nil
			}
		}
	}

	db := repository.DB.Preload("User").Preload("Movie")
	if status != "" {
		db = db.Where("status = ?", status)
	}

	var total int64
	db.Model(&model.Review{}).Count(&total)

	var reviews []model.Review
	// Sắp xếp toxic cao nhất lên trước
	err := db.Order("toxic_score DESC, created_at DESC").Limit(limit).Offset(offset).Find(&reviews).Error

	// Save to Cache (30s)
	if err == nil && redispkg.Client != nil {
		dataReq := struct {
			Reviews []model.Review `json:"reviews"`
			Total   int64          `json:"total"`
		}{reviews, total}
		if data, err := json.Marshal(dataReq); err == nil {
			redispkg.Client.Set(redispkg.Ctx, cacheKey, data, 30*time.Second)
		}
	}

	return reviews, total, err
}

func (s *ReviewService) UpdateReviewStatus(reviewID int, status string) error {
	var review model.Review
	if err := repository.DB.First(&review, reviewID).Error; err != nil {
		return err
	}
	review.Status = model.ReviewStatus(status)
	if err := repository.DB.Save(&review).Error; err != nil {
		return err
	}

	s.invalidateMovieReviewCache(review.MovieID)
	s.invalidateAdminReviewCache()
	return nil
}
