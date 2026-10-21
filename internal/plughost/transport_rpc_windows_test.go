package plughost

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/vmihailenco/msgpack/v5"
)

func TestRPCPluginWindowsPackagedEntrypoint(t *testing.T) {
	if os.Getenv("F4_RPC_PACKAGED_HELPER") == "1" {
		sess := f4rpc.NewSession(os.Stdin, os.Stdout)
		sess.Register("Plugin.Init", func(msgpack.RawMessage) (any, error) {
			return map[string]any{"Drives": []string{"Packaged drive"}}, nil
		})
		_ = sess.Serve()
		os.Exit(0)
	}

	t.Setenv("F4_RPC_PACKAGED_HELPER", "1")
	oldStore := pluginPermissionStore
	pluginPermissionStore = LoadPermissionStore(filepath.Join(t.TempDir(), "permissions.json"))
	t.Cleanup(func() { pluginPermissionStore = oldStore })
	if err := pluginPermissionStore.Remember("packaged-helper", PermissionNative, PermissionAllow); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"android-plugin", "ios-plugin", "cloudfox-plugin"} {
		for _, suffix := range []string{"", ".exe"} {
			t.Run(name+suffix, func(t *testing.T) {
				dir := filepath.Join(t.TempDir(), "plugin with spaces")
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
				copyRPCExecutable(t, filepath.Join(dir, name+suffix))
				plugin := NewRPCPlugRing(dir, name+" -test.run=^TestRPCPluginWindowsPackagedEntrypoint$")
				plugin.SetPermissionIdentity(PluginIdentity{Key: "packaged-helper"})
				api := newLuaTestHostAPI()
				if err := plugin.Init(api); err != nil {
					t.Fatalf("initialize packaged %s: %v", name+suffix, err)
				}
				if api.drives["Packaged drive"] == nil {
					t.Fatal("packaged plugin did not register its drive")
				}
				if err := plugin.Close(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestRPCPluginWindowsExecutablePreparation(t *testing.T) {
	for _, tc := range []struct {
		name, source, existingEXE string
		wantEXE                   bool
	}{
		{name: "missing"},
		{name: "non-native", source: "#!/bin/sh\n"},
		{name: "legacy PE", source: "MZlegacy", wantEXE: true},
		{name: "existing exe", source: "MZold", existingEXE: "MZnew", wantEXE: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "plugin")
			if tc.source != "" {
				if err := os.WriteFile(path, []byte(tc.source), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.existingEXE != "" {
				if err := os.WriteFile(path+".exe", []byte(tc.existingEXE), 0600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := prepareRPCExecutable(path)
			want := path
			if tc.wantEXE {
				want += ".exe"
			}
			if err != nil || got != want {
				t.Fatalf("prepared = %q, %v; want %q", got, err, want)
			}
			if tc.existingEXE != "" {
				data, err := os.ReadFile(got)
				if err != nil || string(data) != tc.existingEXE {
					t.Fatalf("existing exe was overwritten: %q, %v", data, err)
				}
			}
		})
	}
}

func copyRPCExecutable(t *testing.T, dest string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()
	out, err := os.Create(dest)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(out, source)
	closeErr := out.Close()
	if copyErr != nil {
		t.Fatal(copyErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}
