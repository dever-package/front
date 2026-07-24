package middleware

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shemic/dever/server"

	"github.com/dever-package/front/service/siteconfig"
)

const uploadOpenCookiePrefix = "front-token:"

func attachUploadOpenCookieAuthorization(frontConfig siteconfig.Config, c *server.Context) {
	if c == nil || c.Method() != http.MethodGet || strings.TrimSpace(c.Header("Authorization")) != "" {
		return
	}
	if !siteconfig.IsFrontRuntimeAPIEndpoint(c.Path(), "upload/open") {
		return
	}
	token := uploadOpenCookieToken(frontConfig, c)
	if token == "" {
		return
	}
	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return
	}
	raw.Request().Header.Set("Authorization", "Bearer "+token)
}

func uploadOpenCookieToken(frontConfig siteconfig.Config, c *server.Context) string {
	cookies := frontTokenCookies(c.Header("Cookie"))
	if len(cookies) == 0 {
		return ""
	}

	siteKey := "admin"
	if site, ok := requestSite(frontConfig, c, c.Path()); ok && strings.TrimSpace(site.Key) != "" {
		siteKey = site.Key
	}
	host := siteconfig.RequestHost(c.Header("X-Forwarded-Host"), c.Header("Host"))
	expectedName := uploadOpenCookiePrefix + normalizeAuthStorageKey(siteKey) + "_" + normalizeAuthStorageKey(host)
	if token := normalizeUploadOpenCookieToken(cookies[expectedName]); token != "" {
		return token
	}

	hostSuffix := "_" + normalizeAuthStorageKey(host)
	hostMatches := make([]string, 0, 1)
	for name, value := range cookies {
		if strings.HasSuffix(name, hostSuffix) {
			hostMatches = append(hostMatches, value)
		}
	}
	if len(hostMatches) == 1 {
		return normalizeUploadOpenCookieToken(hostMatches[0])
	}
	if len(cookies) == 1 {
		for _, value := range cookies {
			return normalizeUploadOpenCookieToken(value)
		}
	}
	return ""
}

func frontTokenCookies(header string) map[string]string {
	result := make(map[string]string)
	for _, part := range strings.Split(header, ";") {
		name, value, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			continue
		}
		name = decodeCookiePart(name)
		if !strings.HasPrefix(name, uploadOpenCookiePrefix) {
			continue
		}
		result[name] = decodeCookiePart(value)
	}
	return result
}

func decodeCookiePart(value string) string {
	decoded, err := url.QueryUnescape(strings.TrimSpace(value))
	if err != nil {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(decoded)
}

func normalizeUploadOpenCookieToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var decoded string
	if json.Unmarshal([]byte(value), &decoded) == nil {
		value = strings.TrimSpace(decoded)
	}
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		value = strings.TrimSpace(value[len("bearer "):])
	}
	if len(value) == 0 || len(value) > 8192 {
		return ""
	}
	return value
}

func normalizeAuthStorageKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	var result strings.Builder
	separator := false
	for _, char := range value {
		allowed := (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '.' || char == '_' || char == '-'
		if allowed {
			result.WriteRune(char)
			separator = false
			continue
		}
		if !separator {
			result.WriteByte('_')
			separator = true
		}
	}
	if normalized := result.String(); normalized != "" {
		return normalized
	}
	return "default"
}
