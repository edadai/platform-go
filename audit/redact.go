package audit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

var sensitiveKeys = map[string]struct{}{
	"password": {}, "password_hash": {}, "token": {}, "access_token": {}, "refresh_token": {},
	"authorization": {}, "cookie": {}, "secret": {}, "client_secret": {}, "connection_string": {},
	"authorization_code": {}, "card_number": {}, "cvv": {}, "raw_webhook": {}, "document_content": {},
}

// Redact recursively removes well-known secret fields. Producers must still use
// per-action allowlists; this is a defense-in-depth boundary.
func Redact(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSensitiveKey(key) {
				result[key] = "[REDACTED]"
				continue
			}
			result[key] = Redact(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = Redact(item)
		}
		return result
	default:
		return value
	}
}

func Fingerprint(key []byte, value string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func RedactChanges(changes []Change) []Change {
	result := make([]Change, len(changes))
	for index, change := range changes {
		result[index] = change
		if isSensitiveKey(change.Field) {
			result[index].Before = "[REDACTED]"
			result[index].After = "[REDACTED]"
			continue
		}
		result[index].Before = Redact(change.Before)
		result[index].After = Redact(change.After)
	}
	return result
}

func isSensitiveKey(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("-", "_", ".", "_").Replace(value)
	if _, ok := sensitiveKeys[value]; ok {
		return true
	}
	switch strings.ReplaceAll(value, "_", "") {
	case "password", "passwordhash", "token", "accesstoken", "refreshtoken", "authorization",
		"cookie", "secret", "clientsecret", "connectionstring", "authorizationcode",
		"cardnumber", "cvv", "rawwebhook", "documentcontent":
		return true
	}
	for _, suffix := range []string{"_password", "_token", "_secret", "_cookie", "_authorization_code"} {
		if strings.HasSuffix(value, suffix) {
			return true
		}
	}
	return false
}
