package services

import (
	"context"
	"testing"

	"xprem/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Explicit pins may never turn into the next available update when a request
// names an incompatible or unavailable update. Both storage modes share this
// contract; otherwise publish races break a manifest's integrity hashes.
func TestAssetPinsFailClosedAcrossCompatibilityBoundaries(t *testing.T) {
	for _, mode := range []string{"", "postgres://stub"} {
		t.Run("mode="+mode, func(t *testing.T) {
			t.Setenv("DB_URL", mode)
			for _, mismatch := range []string{"platform", "runtime", "channel", "missing", "unchecked", "app"} {
				t.Run(mismatch, func(t *testing.T) {
					h := newRolloutTestHarness(t)
					mapping := &types.ChannelResolution{BranchName: "prod"}
					h.seed(seedRow{branch: "prod", rtv: "1", platform: "ios", id: 100, checked: true})
					params := AssetResolutionParams{AppID: h.appId, ChannelName: "prod", Branch: "prod", RuntimeVersion: "1", Platform: "ios", UpdateID: "100"}
					switch mismatch {
					case "platform":
						params.Platform = "android"
					case "runtime":
						params.RuntimeVersion = "2"
					case "channel":
						mapping.BranchName = "uat"
					case "missing":
						params.UpdateID = "999"
					case "unchecked":
						h.updateRepo.rows[0].checked = false
					case "app":
						params.AppID = "other-app"
					}
					_, served, err := h.protocolService.resolveAssetUpdate(context.Background(), params, mapping)
					require.Error(t, err)
					var protocolErr *ExpoAssetError
					require.ErrorAs(t, err, &protocolErr)
					assert.Equal(t, 404, protocolErr.StatusCode)
					assert.Nil(t, served)
				})
			}
		})
	}
}

func TestUUIDAssetPinsValidateRuntimeAndStoredPlatform(t *testing.T) {
	t.Setenv("DB_URL", "postgres://stub")
	for _, mismatch := range []string{"runtime", "platform"} {
		t.Run(mismatch, func(t *testing.T) {
			h := newRolloutTestHarness(t)
			h.seed(seedRow{branch: "prod", rtv: "1", platform: "ios", id: 100, checked: true, uuid: patchTestRequestedUUID})
			params := AssetResolutionParams{AppID: h.appId, ChannelName: "prod", RuntimeVersion: "1", Platform: "ios", RequestedUpdateID: patchTestRequestedUUID}
			if mismatch == "runtime" {
				params.RuntimeVersion = "2"
			} else {
				params.Platform = "android"
			}
			_, served, err := h.protocolService.resolveAssetUpdate(context.Background(), params, &types.ChannelResolution{BranchName: "prod"})
			require.Error(t, err)
			assert.Nil(t, served)
		})
	}
}

func TestPatchNegotiationOnlySubstitutesMatchingPlatformLaunchHash(t *testing.T) {
	for _, mismatch := range []string{"image", "hash", "platform", "runtime", "source platform"} {
		t.Run(mismatch, func(t *testing.T) {
			h, service, patches := patchHarness(t)
			params := patchParams(h)
			switch mismatch {
			case "image":
				params.Extension = "png"
			case "hash":
				params.Hash = "different-valid-asset-hash"
			case "platform":
				params.Platform = "android"
			case "runtime":
				params.RuntimeVersion = "2"
			case "source platform":
				h.updateRepo.rows[0].platform = "android"
			}
			assert.Nil(t, service.resolveBSDiffAsset(context.Background(), params))
			assert.Zero(t, patches.readCalls)
		})
	}
}

type assetIdentityCountingRepo struct {
	*fakeUpdateRepo
	metadataReads int
	mappingReads  int
}

func (r *assetIdentityCountingRepo) RetrieveUpdateStoredMetadata(ctx context.Context, update types.Update) (*types.UpdateStoredMetadata, error) {
	r.metadataReads++
	return r.fakeUpdateRepo.RetrieveUpdateStoredMetadata(ctx, update)
}

func (r *assetIdentityCountingRepo) GetUpdateAssetMapping(ctx context.Context, update types.Update) (*types.UpdateAssetMapping, error) {
	r.mappingReads++
	return r.fakeUpdateRepo.GetUpdateAssetMapping(ctx, update)
}

func TestPatchIdentityChecksAreCachedAfterFirstDownload(t *testing.T) {
	h, service, _ := patchHarness(t)
	repo := &assetIdentityCountingRepo{fakeUpdateRepo: h.updateRepo}
	service.updateRepo = repo
	for i := 0; i < 3; i++ {
		result, err := service.ResolveAsset(context.Background(), patchParams(h))
		require.NoError(t, err)
		assertPatchResult(t, result)
	}
	assert.Equal(t, 2, repo.metadataReads, "source and target platforms are read once each")
	assert.Equal(t, 1, repo.mappingReads, "the target launch hash is read only once")
}

// Warm every combination to exercise cache hits, including identical branch
// names and update IDs in different apps. An empty channel cannot borrow a
// compatible update from any other channel, platform, runtime or app.
func TestManifestDeliveryIsolatesAppsChannelsPlatformsAndRuntimes(t *testing.T) {
	t.Setenv("DB_URL", "postgres://stub")
	h := newRolloutTestHarness(t)
	apps := []string{h.appId, h.appId + "-second"}
	channels := []string{"dev", "uat", "prod"}
	for _, app := range apps {
		h.appId = app
		for _, channel := range channels {
			h.channelRepo.mappings[channel] = &types.ChannelResolution{BranchName: channel}
			h.seed(seedRow{branch: channel, rtv: "1", platform: "ios", id: 100, checked: true})
		}
	}
	for repeat := 0; repeat < 2; repeat++ {
		for _, app := range apps {
			for _, channel := range channels {
				params := ManifestRequestParams{AppID: app, ChannelName: channel, RuntimeVersion: "1", Platform: "ios"}
				resolved, err := h.protocolService.ResolveUpdateForDevice(context.Background(), params)
				require.NoError(t, err)
				require.NotNil(t, resolved.Update)
				assert.Equal(t, app, resolved.Update.AppId)
				assert.Equal(t, channel, resolved.Update.Branch)
				params.Platform = "android"
				resolved, err = h.protocolService.ResolveUpdateForDevice(context.Background(), params)
				require.NoError(t, err)
				assert.Nil(t, resolved.Update)
				params.Platform, params.RuntimeVersion = "ios", "2"
				resolved, err = h.protocolService.ResolveUpdateForDevice(context.Background(), params)
				require.NoError(t, err)
				assert.Nil(t, resolved.Update)
			}
		}
	}
	h.channelRepo.mappings["empty"] = &types.ChannelResolution{BranchName: "empty"}
	resolved, err := h.protocolService.ResolveUpdateForDevice(context.Background(), ManifestRequestParams{AppID: apps[0], ChannelName: "empty", RuntimeVersion: "1", Platform: "ios"})
	require.NoError(t, err)
	assert.Nil(t, resolved.Update)
	_, err = h.protocolService.ResolveUpdateForDevice(context.Background(), ManifestRequestParams{AppID: apps[0], ChannelName: "missing", RuntimeVersion: "1", Platform: "ios"})
	require.Error(t, err)
}
