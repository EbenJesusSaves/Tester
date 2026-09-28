package update

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"testing"

	"xprem/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Expo exports can reuse an asset path on both platforms. A warm asset cache
// must preserve the requesting platform and role, as well as content hashes.
func TestLegacyManifestAssetCachePreservesPlatformAndRole(t *testing.T) {
	base := setupLocalBucket(t)
	u := types.Update{AppId: t.Name(), Branch: "prod", RuntimeVersion: "1", UpdateId: "100"}
	const content = "shared bytes"
	writeUpdateFile(t, base, u, "assets/shared", content)
	asset := types.Asset{Path: "assets/shared", Ext: "png"}

	for _, platform := range []types.Platform{types.PlatformIOS, types.PlatformAndroid, types.PlatformIOS} {
		for _, launch := range []bool{false, true} {
			shaped, err := shapeManifestAsset(context.Background(), u, &asset, launch, platform)
			require.NoError(t, err)
			parsed, err := url.Parse(shaped.Url)
			require.NoError(t, err)
			assert.Equal(t, string(platform), parsed.Query().Get("platform"))
			assert.Equal(t, u.UpdateId, parsed.Query().Get("updateId"))
			if launch {
				assert.Equal(t, ".bundle", shaped.FileExtension)
				assert.Equal(t, "application/javascript", shaped.ContentType)
			} else {
				assert.Equal(t, ".png", shaped.FileExtension)
				assert.Equal(t, "image/png", shaped.ContentType)
			}
			digest, key := sha256.Sum256([]byte(content)), md5.Sum([]byte(content))
			assert.Equal(t, base64.RawURLEncoding.EncodeToString(digest[:]), shaped.Hash)
			assert.Equal(t, hex.EncodeToString(key[:]), shaped.Key)
		}
	}
}

func TestManifestCachesFollowChangedAssetOrigin(t *testing.T) {
	base := setupLocalBucket(t)
	u := types.Update{AppId: t.Name(), Branch: "prod", RuntimeVersion: "1", UpdateId: "100"}
	writeUpdateFile(t, base, u, "bundle", "bundle")
	firstKey := ComputeManifestResponseCacheKey(u.AppId, u.Branch, u.RuntimeVersion, u.UpdateId, types.PlatformIOS)
	_, err := shapeManifestAsset(context.Background(), u, &types.Asset{Path: "bundle"}, true, types.PlatformIOS)
	require.NoError(t, err)

	t.Setenv("BASE_URL", "https://ota.example.com")
	assert.NotEqual(t, firstKey, ComputeManifestResponseCacheKey(u.AppId, u.Branch, u.RuntimeVersion, u.UpdateId, types.PlatformIOS))
	asset, err := shapeManifestAsset(context.Background(), u, &types.Asset{Path: "bundle"}, true, types.PlatformIOS)
	require.NoError(t, err)
	assert.Contains(t, asset.Url, "https://ota.example.com/assets?")
}

func TestManifestRejectsMissingOrWrongPlatformMetadata(t *testing.T) {
	metadata := &types.UpdateMetadata{}
	mapping := &types.UpdateAssetMapping{}
	for _, stored := range []*types.UpdateStoredMetadata{nil, {Platform: types.PlatformAndroid}} {
		_, err := ComposeUpdateManifest(context.Background(), metadata, types.Update{}, stored, mapping, types.PlatformIOS)
		require.Error(t, err)
	}
	_, err := ComposeUpdateManifest(context.Background(), nil, types.Update{}, &types.UpdateStoredMetadata{Platform: types.PlatformIOS}, mapping, types.PlatformIOS)
	require.Error(t, err)
}
