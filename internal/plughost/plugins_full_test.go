//go:build !lite

package plughost

import "testing"

func TestFullBuildBundledPluginRegistry(t *testing.T) {
	plugins := optionalVFSPlugins()
	t.Cleanup(func() {
		for _, plugin := range plugins {
			_ = plugin.Close()
		}
	})
	for id, name := range map[string]string{
		"android": "Android", "ios": "iOS", "cloudfox": "CloudFox",
	} {
		if !BuiltinPlugRingItem(id) {
			t.Errorf("full build does not identify %s as bundled", id)
		}
		found := false
		for _, plugin := range plugins {
			if plugin.GetName() == name {
				found = true
			}
		}
		if !found {
			t.Errorf("full build does not register %s", name)
		}
	}
}
