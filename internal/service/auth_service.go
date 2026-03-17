package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/booking-show/booking-show-api/pkg/jwt"
	"github.com/booking-show/booking-show-api/pkg/rabbitmq"
	"github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	Cfg          *config.Config
	EmailService *EmailService
}

type RegisterReq struct {
	FullName string `json:"full_name" binding:"required,min=3"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginReq struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (s *AuthService) RegisterUser(req RegisterReq) error {
	var count int64
	repository.DB.Model(&model.User{}).Where("email = ? OR full_name = ?", req.Email, req.FullName).Count(&count)
	if count > 0 {
		return errors.New("email hoặc họ tên này đã được sử dụng")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := model.User{
		FullName:     req.FullName,
		Username:     req.FullName, // Hợp nhất: Dùng FullName làm Username trong DB
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		Role:         model.RoleCustomer,
	}

	if err := repository.DB.Create(&user).Error; err != nil {
		return err
	}

	// 1. Logic Write-through (Redis): Lưu trạng thái tồn tại vào Redis vĩnh viễn
	if redis.Client != nil {
		ctx := context.Background()
		redis.Client.Set(ctx, fmt.Sprintf("exists:email:%s", user.Email), "1", 0)
		redis.Client.Set(ctx, fmt.Sprintf("exists:username:%s", user.Username), "1", 0)
	}

	return nil
}

// CheckAvailability kiểm tra xem email hoặc họ tên đã tồn tại chưa
func (s *AuthService) CheckAvailability(value string, fieldType string) (bool, error) {
	if value == "" {
		return true, nil
	}

	// Chuyển đổi mapping nếu fieldType là "fullname" (từ frontend) sang trường trong DB
	dbField := fieldType
	if fieldType == "fullname" {
		dbField = "full_name"
	}

	cacheKey := fmt.Sprintf("exists:%s:%s", fieldType, value)

	// 1. Đọc đệm (Read-aside): Kiểm tra trong Redis trước
	if redis.Client != nil {
		val, err := redis.Client.Get(context.Background(), cacheKey).Result()
		if err == nil && val == "1" {
			return false, nil // "Hit" -> Đã trung
		}
	}

	// 2. Nếu không thấy trong Redis (Miss) hoặc Redis sập -> Thực hiện truy vấn vào PostgreSQL
	var count int64
	err := repository.DB.Model(&model.User{}).Where(fmt.Sprintf("%s = ?", dbField), value).Count(&count).Error
	if err != nil {
		return false, err // Lỗi DB
	}

	exists := count > 0

	// 3. Nếu PostgreSQL báo trùng, hãy cập nhật lại giá trị đó vào Redis
	if exists && redis.Client != nil {
		redis.Client.Set(context.Background(), cacheKey, "1", 0)
	}

	return !exists, nil // Available if not exists
}

func (s *AuthService) LoginUser(req LoginReq) (*TokenResponse, *model.User, error) {
	var user model.User
	if err := repository.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		return nil, nil, errors.New("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, nil, errors.New("invalid email or password")
	}

	accessExp, _ := time.ParseDuration(s.Cfg.JWTAccessExpiration)
	refreshExp, _ := time.ParseDuration("168h") // 7 days

	access, refresh, err := jwt.GenerateTokens(user.ID, string(user.Role), s.Cfg.JWTSecret, accessExp, refreshExp)
	if err != nil {
		return nil, nil, err
	}

	return &TokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
	}, &user, nil
}

func (s *AuthService) RequestMagicLink(email string) error {
	var user model.User
	if err := repository.DB.Where("email = ?", email).First(&user).Error; err != nil {
		return errors.New("không tìm thấy người dùng với email này")
	}

	token := uuid.New().String()
	key := fmt.Sprintf("magic_link:%s", token)

	// Lưu vào Redis với TTL 15 phút
	err := redis.Client.Set(context.Background(), key, email, 15*time.Minute).Err()
	if err != nil {
		return fmt.Errorf("không thể lưu token vào hệ thống: %v", err)
	}

	// Gửi email
	// 4. Publish Event "Gửi email" vào RabbitMQ thay vì gọi SendMail đồng bộ (Blocking)
	// if s.EmailService == nil {
	// 	s.EmailService = NewEmailService(s.Cfg)
	// }
	// if err := s.EmailService.SendMagicLink(email, token); err != nil {
	// 	return err
	// }
	emailEvent := map[string]string{
		"email": user.Email,
		"token": token,
	}
	eventBody, _ := json.Marshal(emailEvent)
	err = rabbitmq.PublishMessage("email.send_magic_link", eventBody)

	if err != nil {
		return &AppError{Code: "EMAIL_FAILED", Status: http.StatusInternalServerError, Msg: "Không thể đưa email vào hàng đợi."}
	}

	return nil
}

func (s *AuthService) VerifyMagicLink(token string) (*TokenResponse, *model.User, error) {
	key := fmt.Sprintf("magic_link:%s", token)
	email, err := redis.Client.Get(context.Background(), key).Result()
	if err != nil {
		return nil, nil, errors.New("link không hợp lệ hoặc đã hết hạn")
	}

	// Xóa token sau khi dùng
	redis.Client.Del(context.Background(), key)

	var user model.User
	if err := repository.DB.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, nil, errors.New("không tìm thấy thông tin người dùng")
	}

	accessExp, _ := time.ParseDuration(s.Cfg.JWTAccessExpiration)
	refreshExp, _ := time.ParseDuration("168h")

	access, refresh, err := jwt.GenerateTokens(user.ID, string(user.Role), s.Cfg.JWTSecret, accessExp, refreshExp)
	if err != nil {
		return nil, nil, err
	}

	return &TokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
	}, &user, nil
}

func (s *AuthService) OAuthLoginUser(email, fullName, provider, providerID string) (*TokenResponse, *model.User, error) {
	var user model.User
	
	// Tìm user theo email hoặc provider id
	err := repository.DB.Where("email = ? OR (provider = ? AND provider_id = ?)", email, provider, providerID).First(&user).Error
	
	if err != nil {
		// Nếu không tìm thấy, tạo user mới
		user = model.User{
			Email:      email,
			FullName:   fullName,
			Username:   fullName,
			Provider:   provider,
			ProviderID: providerID,
			Role:       model.RoleCustomer,
			IsActive:   true,
		}
		
		if err := repository.DB.Create(&user).Error; err != nil {
			return nil, nil, fmt.Errorf("không thể tạo người dùng mới: %v", err)
		}

		// Cập nhật Cache Redis
		if redis.Client != nil {
			ctx := context.Background()
			redis.Client.Set(ctx, fmt.Sprintf("exists:email:%s", user.Email), "1", 0)
			redis.Client.Set(ctx, fmt.Sprintf("exists:username:%s", user.Username), "1", 0)
		}
	} else {
		// Nếu đã tìm thấy, cập nhật ProviderID nếu trước đó chưa có (trường hợp user reg bằng email trước đó)
		if user.ProviderID == "" {
			repository.DB.Model(&user).Updates(model.User{Provider: provider, ProviderID: providerID})
		}
	}

	// Tạo Token
	accessExp, _ := time.ParseDuration(s.Cfg.JWTAccessExpiration)
	refreshExp, _ := time.ParseDuration("168h")

	access, refresh, err := jwt.GenerateTokens(user.ID, string(user.Role), s.Cfg.JWTSecret, accessExp, refreshExp)
	if err != nil {
		return nil, nil, err
	}

	return &TokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
	}, &user, nil
}

func (s *AuthService) ResetPassword(token, newPassword string) error {
	key := fmt.Sprintf("magic_link:%s", token)
	email, err := redis.Client.Get(context.Background(), key).Result()
	if err != nil {
		return errors.New("link không hợp lệ hoặc đã hết hạn")
	}

	// Hash mật khẩu mới
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// Cập nhật Database
	result := repository.DB.Model(&model.User{}).Where("email = ?", email).Update("password_hash", string(hashedPassword))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("không tìm thấy người dùng để cập nhật")
	}

	// Xóa token sau khi dùng
	redis.Client.Del(context.Background(), key)

	return nil
}
