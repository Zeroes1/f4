package plughost

// f4's first-party PlugRing catalog: native plugins this repository builds,
// tests and releases itself, offered through the same PlugRing "download and
// install in one click" UI as a community entry, but never through
// plugring/index.yaml or any remote community catalog.
//
// Why a separate list rather than an entry in plugring/index.yaml: PLUGRING.md
// and PlugRingItemProblem refuse a per-platform download URL ({os}/{arch}) and
// an entrypoint that is not a bare .lua or .wasm file, because a platform
// binary submitted to a community catalog is a binary no distribution will
// mirror, no reviewer can audit, and no user can check. That is a real threat
// -- but it is a threat about a stranger's binary. cloudfox-plugin is not a
// stranger's binary: it is built by this repository's own CI
// (.github/workflows/build.yml's build-cloudfox-plugin job) from code that
// went through this repository's own PR review, and published as this
// repository's own GitHub Release asset (f4#1178 part 2 of 4). Treating it
// exactly like an unreviewed third-party submission would not add any safety;
// it would only stop f4 from offering "install cloudfox" through the UI it
// already has for everything else.
//
// So this file is the one and only place a PlugRingItem gets FirstParty:
// true. PlugRingItem.FirstParty is tagged json:"-" and yaml:"-", so nothing
// decoded from plugring/index.yaml or a remote catalog -- i.e. nothing a pull
// request to plugring/ could ever write -- can set it; the community
// distribution policy in PlugRingItemProblem keeps applying in full to every
// entry that does not come from here.
//
// f4#1178 part 3 of 4 (plan: issuecomment-5851218447 on #1178). The same
// reasoning applies verbatim to ios-plugin (plugins/ios/cmd/ios-plugin),
// added below as this catalog's second entry once iOS got the same
// downloadable-plugin treatment as cloud storage
// (issuecomment-5851392645); see iosPlugRingVersion's own comment.

// cloudFoxPlugRingVersion mirrors plugins/cloudfox/cmd/cloudfox-plugin/
// plugring-manifest.json's version field by hand: that manifest lives in
// plugins/cloudfox's own Go module (split out in part 1), which this
// package's module cannot import or go:embed across the module boundary, so
// the two are kept in sync by comment rather than by code sharing. Keep them
// matching when either changes.
//
// The value stays an honest "not installable yet" placeholder until a tagged
// release actually carries cloudfox-plugin-{os}-{arch}.tar.gz assets: as of
// part 3, plugring-manifest.json's URL resolves against
// releases/latest/download/, and no release has shipped that asset yet, so
// installing this entry today 404s. Wiring the download/install path
// (this file, and the PlugRingItemProblem/UI changes alongside it) does not
// by itself make the asset exist -- see f4#1178 part 4 and later.
const cloudFoxPlugRingVersion = "0.0.0-part3-published-not-installable"

// iosPlugRingVersion mirrors plugins/ios/cmd/ios-plugin/plugring-manifest.json's
// version field by hand, for the same reason cloudFoxPlugRingVersion does:
// that manifest lives in plugins/ios's own Go module (split out in f4#1178
// part 1), which this package's module cannot import or go:embed across the
// module boundary, so the two are kept in sync by comment rather than by
// code sharing. Keep them matching when either changes.
//
// The value stays an honest "not installable yet" placeholder until a
// tagged release actually carries ios-plugin-{os}-{arch}.tar.gz assets:
// plugring-manifest.json's URL resolves against releases/latest/download/
// (f4#1178 part 2), and no release has shipped that asset yet, so
// installing this entry today 404s. Wiring the download/install path (this
// file, and the entry below) does not by itself make the asset exist.
const iosPlugRingVersion = "0.0.0-part2-published-not-installable"

// FirstPartyPlugRingItems returns f4's own first-party PlugRing catalog. It
// is a plain function, not a package-level var, so nothing outside this file
// can mutate the shared list a caller got back from an earlier call.
func FirstPartyPlugRingItems() []PlugRingItem {
	return []PlugRingItem{
		{
			ID:      "cloudfox",
			Name:    "Cloud storage (CloudFox)",
			Version: cloudFoxPlugRingVersion,
			Author:  "unxed",
			Description: "S3, Google Drive, Yandex Disk and WebDAV as a top-level f4 drive. " +
				"Ships as a native subprocess plugin so a lite build (f4#1178) does not have " +
				"to carry the cloud SDKs in its own binary.",
			// {os}/{arch}: a distribution policy violation in the community
			// catalog (PLUGRING.md), and exactly what FirstParty exempts this
			// entry from in PlugRingItemProblem. See build-cloudfox-plugin in
			// .github/workflows/build.yml for how each platform's asset gets
			// this exact name.
			URL:        "https://github.com/unxed/f4/releases/latest/download/cloudfox-plugin-{os}-{arch}.tar.gz",
			Entrypoint: "cloudfox-plugin",
			Category:   PlugRingCategoryFilesystem,
			Runtimes:   []string{PlugRingRuntimeNative},
			FirstParty: true,
		},
		{
			ID:      "ios",
			Name:    "Apple mobile devices (iOS)",
			Version: iosPlugRingVersion,
			Author:  "unxed",
			Description: "Browse an iPhone or iPad's Media export, applications and crash " +
				"reports as a top-level f4 drive over usbmuxd. Ships as a native subprocess " +
				"plugin so a lite build (f4#1178) does not have to carry go-ios and its " +
				"userspace networking stack in its own binary.",
			// {os}/{arch}: a distribution policy violation in the community
			// catalog (PLUGRING.md), and exactly what FirstParty exempts this
			// entry from in PlugRingItemProblem. See build-ios-plugin in
			// .github/workflows/build.yml for how each platform's asset gets
			// this exact name.
			URL:        "https://github.com/unxed/f4/releases/latest/download/ios-plugin-{os}-{arch}.tar.gz",
			Entrypoint: "ios-plugin",
			Category:   PlugRingCategoryFilesystem,
			Runtimes:   []string{PlugRingRuntimeNative},
			FirstParty: true,
		},
	}
}

// MergeFirstPartyPlugRingItems appends f4's first-party catalog
// (FirstPartyPlugRingItems) to a fetched community catalog, so the PlugRing
// dialog can build its rows from one combined list instead of two.
//
// A community entry whose id collides with a first-party one is dropped in
// favour of the first-party entry: the first-party list is the one this
// repository vouches for under that name, and letting a same-named community
// entry win would let an unreviewed URL ride in under a name the user has
// already learned to trust. Note that a community entry cannot forge
// FirstParty itself either way (see PlugRingItem.FirstParty); this only
// covers a plain id collision.
//
// The result is run back through NormalizePlugRingCatalog, so category and
// runtime defaults are filled in for first-party entries exactly as they are
// for community ones.
func MergeFirstPartyPlugRingItems(community []PlugRingItem) []PlugRingItem {
	firstParty := FirstPartyPlugRingItems()
	firstPartyIDs := make(map[string]bool, len(firstParty))
	for _, item := range firstParty {
		firstPartyIDs[item.ID] = true
	}

	merged := make([]PlugRingItem, 0, len(community)+len(firstParty))
	for _, item := range community {
		if firstPartyIDs[item.ID] {
			continue
		}
		merged = append(merged, item)
	}
	merged = append(merged, firstParty...)
	return NormalizePlugRingCatalog(merged)
}
