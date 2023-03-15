package webhook

import (
	"os"

	"github.com/sirupsen/logrus"
)

// HMACToken gets the HMAC token from the environment or filesystem.
func HMACToken() string {
	hmacToken := os.Getenv("HMAC_TOKEN")
	// For backwards compatibility we only attempt to read from the filesystem
	// if the HMAC token is not set in the environment
	if len(hmacToken) == 0 {
		// If HMAC_TOKEN_PATH is specified then attempt to read from the filesystem
		hmacTokenPath := os.Getenv("HMAC_TOKEN_PATH")
		if len(hmacTokenPath) > 0 {
			b, err := os.ReadFile(hmacTokenPath)
			if err != nil {
				logrus.Errorf("failed to read HMAC_TOKEN_PATH %s: %s", hmacTokenPath, err)
				return hmacToken
			}
			hmacToken = string(b)
		}
	}
	return hmacToken
}
