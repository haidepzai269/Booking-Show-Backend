package repository

import (
	"log"

	"github.com/booking-show/booking-show-api/internal/model"
)

// MigrateDB create tables
func MigrateDB() {
	err := DB.AutoMigrate(
		&model.User{},
		&model.Cinema{},
		&model.Room{},
		&model.Seat{},
		&model.Genre{},
		&model.Movie{},
		&model.Concession{},
		&model.Promotion{},
		&model.Showtime{},
		&model.ShowtimeSeat{},
		&model.Order{},
		&model.OrderConcession{},
		&model.OrderSeat{},
		&model.Payment{},
		&model.Ticket{},
		&model.Person{},
		&model.RefundRequest{},
		&model.FAQLog{},
		&model.Campaign{},
		&model.ChatHistory{},
		&model.NewsletterSubscription{},
		&model.Notification{},
		&model.Review{},
		&model.ReviewLike{},
		&model.BlacklistedWord{},
	)

	if err != nil {
		log.Fatalf("Failed to auto migrate database: %v", err)
	}

	// Sửa lỗi cho dữ liệu cũ: Điền username bằng phần prefix của email nếu username đang trống
	DB.Exec(`UPDATE users SET username = SPLIT_PART(email, '@', 1) || '_' || id WHERE username IS NULL OR username = '';`)

	// Sau khi đã điền dữ liệu, ta có thể thêm ràng buộc NOT NULL thủ công nếu muốn, 
	// hoặc để AutoMigrate xử lý ở lần sau bằng cách thêm lại tag not null vào model.
	DB.Exec(`ALTER TABLE users ALTER COLUMN username SET NOT NULL;`)

	// Tạo Index GIN với pg_trgm cho Users và Movies
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_users_full_name_trgm ON users USING gin (full_name gin_trgm_ops);`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_users_username_trgm ON users USING gin (username gin_trgm_ops);`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_users_email_trgm ON users USING gin (email gin_trgm_ops);`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_movies_title_trgm ON movies USING gin (title gin_trgm_ops);`)

	// Cập nhật kích thước vector sang 1024 (Voyage AI) - Cần thực hiện thủ công vì AutoMigrate không đổi size cột vector
	// Lưu ý: Xóa dữ liệu cũ vì không tương thích số chiều
	DB.Exec(`UPDATE movies SET embedding = NULL;`)
	DB.Exec(`ALTER TABLE movies ALTER COLUMN embedding TYPE vector(1024);`)
	DB.Exec(`ALTER TABLE concessions ADD COLUMN IF NOT EXISTS embedding vector(1024);`)
	DB.Exec(`ALTER TABLE promotions ADD COLUMN IF NOT EXISTS embedding vector(1024);`)
	DB.Exec(`ALTER TABLE faq_logs ADD COLUMN IF NOT EXISTS embedding vector(1024);`)

	// Tạo HNSW Index cho Vector Search (pgvector) để tìm kiếm cực nhanh trên tập dữ liệu lớn
	// Lưu ý: Cần xóa index cũ nếu thay đổi số chiều vector
	DB.Exec(`DROP INDEX IF EXISTS idx_movies_embedding_hnsw;`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_movies_embedding_hnsw ON movies USING hnsw (embedding vector_cosine_ops);`)
	
	DB.Exec(`DROP INDEX IF EXISTS idx_concessions_embedding_hnsw;`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_concessions_embedding_hnsw ON concessions USING hnsw (embedding vector_cosine_ops);`)
	
	DB.Exec(`DROP INDEX IF EXISTS idx_promotions_embedding_hnsw;`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_promotions_embedding_hnsw ON promotions USING hnsw (embedding vector_cosine_ops);`)
	
	DB.Exec(`DROP INDEX IF EXISTS idx_faq_logs_embedding_hnsw;`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_faq_logs_embedding_hnsw ON faq_logs USING hnsw (embedding vector_cosine_ops);`)

	log.Println("Database migration completed!")
}
