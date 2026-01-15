package auth

import (
	"fmt"
	"regexp"
)

var deviceIDRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{10,128}$`)

// ValidateDeviceIDFormat checks if the device ID format is valid (base64url/uuid-like).
func ValidateDeviceIDFormat(deviceID string) bool {
	return deviceIDRegex.MatchString(deviceID)
}

// ValidateDeviceID checks if the provided device ID matches the SHA-256 hash of the public Key JWK.
func ValidateDeviceID(deviceID string, pubJWK map[string]interface{}) error {
	if deviceID == "" {
		return fmt.Errorf("device_id is required")
	}
	if !ValidateDeviceIDFormat(deviceID) {
		return fmt.Errorf("invalid device_id format")
	}
	if pubJWK == nil {
		return fmt.Errorf("public_key is required")
	}

	if _, _, err := ParseECPublicJWKMap(pubJWK); err != nil {
		return fmt.Errorf("invalid public key")
	}

	return nil
}
