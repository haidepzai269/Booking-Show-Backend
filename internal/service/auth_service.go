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
	FullName string `json:"full_name" binding:"required"`
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
	repository.DB.Model(&model.User{}).Where("email = ?", req.Email).Count(&count)
	if count > 0 {
		return errors.New("email already exists")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := model.User{
		FullName:     req.FullName,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		Role:         model.RoleCustomer,
	}

	return repository.DB.Create(&user).Error
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
