package assets

import (
	"context"
	"testing"

	"xprem/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetCachePolicySeparatesHeaderRoutedApps(t *testing.T) {
	for _, pinned := range []bool{true, false} {
		headers := legacyAssetHeaders(AssetsRequest{Pinned: pinned})
		assert.Contains(t, headers["Vary"], "Expo-App-ID")
		assert.Contains(t, headers["Vary"], "Expo-Channel-Name")
		assert.Contains(t, headers["Vary"], "Accept-Encoding")
		if pinned {
			assert.Contains(t, headers["Cache-Control"], "immutable")
		} else {
			assert.Equal(t, "private, no-store", headers["Cache-Control"])
		}
	}
}

func TestAssetDeliveryRejectsMismatchedRepositoryHandle(t *testing.T) {
	for _, mismatch := range []string{"app", "branch", "runtime"} {
		t.Run(mismatch, func(t *testing.T) {
			u := &types.Update{AppId: "app", Branch: "prod", RuntimeVersion: "1", UpdateId: "100"}
			req := AssetsRequest{AppId: "app", Branch: "prod", RuntimeVersion: "1", AssetName: "bundle.js", Update: u}
			switch mismatch {
			case "app":
				u.AppId = "other"
			case "branch":
				u.Branch = "uat"
			case "runtime":
				u.RuntimeVersion = "2"
			}
			_, response := validateAssetRequest(context.Background(), req)
			require.NotNil(t, response)
			assert.Equal(t, 404, response.StatusCode)
		})
	}
}
