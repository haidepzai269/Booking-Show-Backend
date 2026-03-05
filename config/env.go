package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DBUrl               string
	RedisUrl            string
	RabbitMQUrl         string
	JWTSecret           string
	JWTAccessExpiration string
	CLOUDINARY_URL      string
	Port                string
	SMTPHost            string
	SMTPPort            string
	SMTPUser            string
	SMTPPass            string
	SMTPFrom            string
	FrontendURL         string
	// VNPay
	VNPayTmnCode    string
	VNPayHashSecret string
	VNPayURL        string
	// ZaloPay
	ZaloPayAppID  string
	ZaloPayKey1   string
	ZaloPayKey2   string
	ZaloPayAPIURL string
	// PayOS
	PayOSClientID    string
	PayOSAPIKey      string
	PayOSChecksumKey string
	// AI
	GroqAPIKey string
}

// LoadEnv loads environment variables from .env file
func LoadEnv() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: No .env file found, using system environment variables")
	}

	appEnv := getEnv("APP_ENV", "cloud")

	dbUrl := getEnv("DB_URL", "")
	redisUrl := getEnv("REDIS_URL", "")
	rabbitMQUrl := getEnv("RABBITMQ_URL", "")

	if appEnv == "local" {
		dbUrl = getEnv("LOCAL_DB_URL", dbUrl)
		redisUrl = getEnv("LOCAL_REDIS_URL", redisUrl)
		rabbitMQUrl = getEnv("LOCAL_RABBITMQ_URL", rabbitMQUrl)
		log.Println("🚀 Running in LOCAL DOCKER environment")
	} else {
		log.Println("🌍 Running in CLOUD environment")
	}

	return &Config{
		DBUrl:               dbUrl,
		RedisUrl:            redisUrl,
		RabbitMQUrl:         rabbitMQUrl,
		JWTSecret:           getEnv("JWT_SECRET", "super-secret-key-change-in-production"),
		JWTAccessExpiration: getEnv("JWT_ACCESS_EXPIRATION", "15m"),
		CLOUDINARY_URL:      getEnv("CLOUDINARY_URL", ""),
		Port:                getEnv("PORT", "8080"),
		SMTPHost:            getEnv("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:            getEnv("SMTP_PORT", "587"),
		SMTPUser:            getEnv("SMTP_USER", ""),
		SMTPPass:            getEnv("SMTP_PASS", ""),
		SMTPFrom:            getEnv("SMTP_FROM", ""),
		FrontendURL:         getEnv("FRONTEND_URL", "http://localhost:3000"),
		// VNPay
		VNPayTmnCode:    getEnv("VNPAY_TMN_CODE", "BOOKSHOW"),
		VNPayHashSecret: getEnv("VNPAY_HASH_SECRET", ""),
		VNPayURL:        getEnv("VNPAY_URL", "https://sandbox.vnpayment.vn/paymentv2/vpcpay.html"),
		// ZaloPay
		ZaloPayAppID:  getEnv("ZALOPAY_APP_ID", ""),
		ZaloPayKey1:   getEnv("ZALOPAY_KEY1", ""),
		ZaloPayKey2:   getEnv("ZALOPAY_KEY2", ""),
		ZaloPayAPIURL: getEnv("ZALOPAY_API_URL", "https://sb-openapi.zalopay.vn/v2/create"),
		// PayOS
		PayOSClientID:    getEnv("PAYOS_CLIENT_ID", ""),
		PayOSAPIKey:      getEnv("PAYOS_API_KEY", ""),
		PayOSChecksumKey: getEnv("PAYOS_CHECKSUM_KEY", ""),
		// AI
		GroqAPIKey: getEnv("GROQ_API_KEY", ""),
	}
}

func getEnv(key, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}
