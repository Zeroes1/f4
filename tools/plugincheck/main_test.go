package main

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/vmihailenco/msgpack/v5"
)

func TestPluginCheck(t *testing.T) {
	if mode := os.Getenv("F4_PLUGIN_CHECK_HELPER"); mode != "" {
		if mode == "hang" {
			time.Sleep(time.Minute)
			os.Exit(1)
		}
		session := f4rpc.NewSession(os.Stdin, os.Stdout)
		session.Register("Plugin.Init", func(msgpack.RawMessage) (any, error) {
			if mode == "profile" {
				var profile struct {
					Dir      string
					Portable bool
				}
				if err := session.Call("Host.GetProfile", nil, &profile); err != nil {
					return nil, err
				}
				info, err := os.Stat(profile.Dir)
				if err != nil || !info.IsDir() || !profile.Portable {
					return nil, errors.New("smoke profile is unavailable or not isolated")
				}
			}
			if mode == "fail" {
				return nil, errors.New("plugin is broken")
			}
			if mode == "legacy" {
				return []string{"Test drive"}, nil
			}
			return map[string]any{"Drives": []string{"Test drive"}}, nil
		})
		session.Register("VFS.ReadDir", func(msgpack.RawMessage) (any, error) {
			if mode == "empty-root" {
				return nil, nil
			}
			return []map[string]any{{"Name": "Device"}}, nil
		})
		_ = session.Serve()
		os.Exit(0)
	}
	for _, tc := range []struct{ mode, expected, wantError string }{
		{"ok", "Test drive", ""},
		{"legacy", "Test drive", ""},
		{"profile", "Test drive", ""},
		{"root", "Test drive", ""},
		{"empty-root", "Test drive", ""},
		{"ok", "Wrong drive", "do not include"},
		{"fail", "", "plugin is broken"},
		{"hang", "", "context deadline exceeded"},
	} {
		t.Run(tc.mode+tc.expected, func(t *testing.T) {
			t.Setenv("F4_PLUGIN_CHECK_HELPER", tc.mode)
			timeout := 10 * time.Second
			if tc.mode == "hang" {
				timeout = 200 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			listRoot := tc.mode == "root" || tc.mode == "empty-root"
			err := checkPlugin(ctx, os.Args[0], []string{"-test.run=^TestPluginCheck$"}, tc.expected, listRoot, io.Discard)
			if tc.wantError == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
		})
	}
}
