package model

import "time"

// Campaign — Chiến dịch marketing/khuyến mãi hiển thị trên trang /promotions
// Khác với Promotion (mã giảm giá), Campaign là nội dung bài viết giới thiệu ưu đãi của đối tác
type CampaignType string
type CampaignStatus string

const (
	CampaignTypeBank    CampaignType = "BANK"
	CampaignTypeWallet  CampaignType = "WALLET"
	CampaignTypePartner CampaignType = "PARTNER"
	CampaignTypeMember  CampaignType = "MEMBER"
	CampaignTypeOther   CampaignType = "OTHER"
)

const (
	CampaignStatusActive   CampaignStatus = "ACTIVE"
	CampaignStatusInactive CampaignStatus = "INACTIVE"
	CampaignStatusDraft    CampaignStatus = "DRAFT"
)

type Campaign struct {
	ID              int            `json:"id" gorm:"primaryKey;autoIncrement"`
	Title           string         `json:"title" gorm:"type:varchar(255);not null"`
	Description     string         `json:"description" gorm:"type:text"`
	ThumbnailURL    string         `json:"thumbnail_url" gorm:"type:varchar(500)"`
	BannerURL       string         `json:"banner_url" gorm:"type:varchar(500)"`
	Type            CampaignType   `json:"type" gorm:"type:varchar(20);default:'OTHER'"`
	HowToAvail      string         `json:"how_to_avail" gorm:"type:text"`     // Hướng dẫn sử dụng (có thể là HTML/Markdown)
	TermsConditions string         `json:"terms_conditions" gorm:"type:text"` // Điều khoản & điều kiện
	StartDate       *time.Time     `json:"start_date"`
	EndDate         *time.Time     `json:"end_date"`
	Status          CampaignStatus `json:"status" gorm:"type:varchar(20);default:'DRAFT'"`
	SortOrder       int            `json:"sort_order" gorm:"default:0"` // Thứ tự hiển thị
	CreatedAt       time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
}
