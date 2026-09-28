package update

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"xprem/internal/bucket"
	"xprem/internal/cache"
	"xprem/internal/types"
)

// Measures cold metadata composition with a warm filesystem. Allocation should
// remain approximately constant as file size grows; this is not a fleet benchmark.
func BenchmarkLegacyAssetHashing(b *testing.B) {
	for _, size := range []int{64 << 10, 1 << 20, 8 << 20} {
		b.Run(fmt.Sprintf("bytes_%d", size), func(b *testing.B) {
			base := b.TempDir()
			b.Setenv("STORAGE_MODE", "local")
			b.Setenv("LOCAL_BUCKET_BASE_PATH", base)
			b.Setenv("BASE_URL", "https://ota.example.test")
			b.Setenv("CACHE_MODE", "local")
			bucket.ResetBucketInstance()
			b.Cleanup(bucket.ResetBucketInstance)
			u := types.Update{AppId: "benchmark", Branch: "dev", RuntimeVersion: "1", UpdateId: "100"}
			file := filepath.Join(base, u.AppId, u.Branch, u.RuntimeVersion, u.UpdateId, "bundle.js")
			if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
				b.Fatal(err)
			}
			if err := os.WriteFile(file, bytes.Repeat([]byte("x"), size), 0o600); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(size))
			b.ReportAllocs()
			for b.Loop() {
				if err := cache.GetCache().Clear(); err != nil {
					b.Fatal(err)
				}
				if _, err := shapeManifestAsset(context.Background(), u, &types.Asset{Path: "bundle.js", Ext: "js"}, true, types.PlatformIOS); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
