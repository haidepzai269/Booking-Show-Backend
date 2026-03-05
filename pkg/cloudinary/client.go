package cloudinary

import (
	"log"

	"github.com/booking-show/booking-show-api/config"
	"github.com/cloudinary/cloudinary-go/v2"
)

var CloudinaryClient *cloudinary.Cloudinary

func ConnectCloudinary(cfg *config.Config) {
	c, err := cloudinary.NewFromURL(cfg.CLOUDINARY_URL)
	if err != nil {
		log.Fatalf("Failed to intialize Cloudinary: %v", err)
	}
	CloudinaryClient = c
	log.Println("Cloudinary configured successfully!")
}
