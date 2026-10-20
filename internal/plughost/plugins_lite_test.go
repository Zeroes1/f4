//go:build lite

package plughost

import "testing"

func TestLiteBuildDownloadsOptionalPlugins(t *testing.T) {
	for _, id := range []string{"android", "ios", "cloudfox"} {
		if BuiltinPlugRingItem(id) {
			t.Errorf("lite build identifies downloadable %s as bundled", id)
		}
	}
}
