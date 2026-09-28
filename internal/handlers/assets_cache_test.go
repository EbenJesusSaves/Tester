package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAssetErrorsCannotBecomeSharedCacheEntries(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/assets?platform=unsupported", nil)
	request.Header.Set("expo-app-id", "app")
	response := httptest.NewRecorder()
	(&ExpoProtocolHandler{}).HandleAssets(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
	assert.Contains(t, response.Header().Get("Vary"), "Expo-App-ID")
	assert.Contains(t, response.Header().Get("Vary"), "Accept-Encoding")
}
