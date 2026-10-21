// Command plugincheck checks a packaged subprocess plugin's Plugin.Init response.
// Usage: plugincheck [-list] path/to/plugin [expected-drive]
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/vmihailenco/msgpack/v5"
)

func main() {
	listRoot := flag.Bool("list", false, "also list the first drive's root (may require attached devices)")
	flag.Parse()
	if flag.NArg() < 1 || flag.NArg() > 2 {
		fmt.Fprintln(os.Stderr, "usage: plugincheck [-list] executable [expected-drive]")
		os.Exit(2)
	}
	expected := ""
	if flag.NArg() == 2 {
		expected = flag.Arg(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := checkPlugin(ctx, flag.Arg(0), nil, expected, *listRoot, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Plugin.Init OK: %s\n", flag.Arg(0))
}

func checkPlugin(ctx context.Context, path string, args []string, expected string, listRoot bool, stderr io.Writer) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	profileDir, err := os.MkdirTemp("", "f4-plugincheck-profile-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(profileDir) }()
	// #nosec G204 G702 -- checking an explicitly supplied release executable is this tool's purpose.
	cmd := exec.CommandContext(ctx, absolute, args...)
	cmd.Dir = filepath.Dir(absolute)
	cmd.Stderr = stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	encoder := msgpack.NewEncoder(stdin)
	if err := encoder.Encode(&f4rpc.Message{ID: 1, Method: "Plugin.Init"}); err != nil {
		return fmt.Errorf("send Plugin.Init: %w", err)
	}
	decoder := msgpack.NewDecoder(stdout)
	requestID := uint32(1)
	method := "Plugin.Init"
	for {
		var reply f4rpc.Message
		if err := decoder.Decode(&reply); err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("%s: %w", method, ctx.Err())
			}
			return fmt.Errorf("read %s: %w", method, err)
		}
		if reply.Type == 0 {
			response := f4rpc.Message{Type: 1, ID: reply.ID}
			switch reply.Method {
			case "Host.Log":
			case "Host.GetProfile":
				data, err := msgpack.Marshal(struct {
					Dir      string
					Portable bool
				}{profileDir, true})
				if err != nil {
					return err
				}
				response.Data = data
			default:
				response.Error = "host method unavailable in smoke test"
			}
			if err := encoder.Encode(&response); err != nil {
				return err
			}
			continue
		}
		if reply.ID != requestID {
			return fmt.Errorf("unexpected response ID %d", reply.ID)
		}
		if reply.Error != "" {
			return fmt.Errorf("%s: %s", method, reply.Error)
		}
		if requestID == 2 {
			var entries []struct{ Name string }
			if len(reply.Data) > 0 {
				if err := msgpack.Unmarshal(reply.Data, &entries); err != nil {
					return fmt.Errorf("decode VFS.ReadDir: %w", err)
				}
			}
			_, err := fmt.Fprintf(stderr, "VFS.ReadDir OK: %d entries\n", len(entries))
			return err
		}
		var result struct{ Drives []string }
		if err := msgpack.Unmarshal(reply.Data, &result); err != nil {
			// The original SDK returns a bare drive list; command-capable
			// plugins use the structured response instead.
			if legacyErr := msgpack.Unmarshal(reply.Data, &result.Drives); legacyErr != nil {
				return fmt.Errorf("decode Plugin.Init: %w", err)
			}
		}
		if len(result.Drives) == 0 || expected != "" && !slices.Contains(result.Drives, expected) {
			return fmt.Errorf("Plugin.Init drives %q do not include %q", result.Drives, expected)
		}
		if listRoot {
			data, err := msgpack.Marshal(struct{ Drive, Path string }{result.Drives[0], "/"})
			if err != nil {
				return err
			}
			requestID = 2
			method = "VFS.ReadDir"
			if err := encoder.Encode(&f4rpc.Message{ID: requestID, Method: method, Data: data}); err != nil {
				return err
			}
			continue
		}
		return nil
	}
}
