package oauth

import (
	"github.com/booking-show/booking-show-api/config"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/facebook"
	"golang.org/x/oauth2/google"
)

var (
	GoogleOauthConfig   *oauth2.Config
	FacebookOauthConfig *oauth2.Config
)

func InitOAuth(cfg *config.Config) {
	GoogleOauthConfig = &oauth2.Config{
		RedirectURL:  cfg.GoogleRedirectURL,
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}

	FacebookOauthConfig = &oauth2.Config{
		RedirectURL:  cfg.FacebookRedirectURL,
		ClientID:     cfg.FacebookClientID,
		ClientSecret: cfg.FacebookClientSecret,
		Scopes:       []string{"email", "public_profile"},
		Endpoint:     facebook.Endpoint,
	}
}
