package cloudfox

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/sdk/f4plugin"
	"golang.org/x/net/webdav"
)

type rpcTestPrompter struct {
	answers  map[string]string
	requests []f4plugin.PromptRequest
	cancel   string
}

func (h *rpcTestPrompter) Prompt(request f4plugin.PromptRequest) (f4plugin.PromptResponse, error) {
	h.requests = append(h.requests, request)
	if request.Prompt == h.cancel {
		return f4plugin.PromptResponse{}, nil
	}
	value, ok := h.answers[request.Prompt]
	if !ok {
		value = request.Default
	}
	return f4plugin.PromptResponse{Text: value, Accepted: true}, nil
}

func TestRPCCloudProfileCreationAndCancellation(t *testing.T) {
	store := &memorySecretStore{}
	p := NewRPCPlugin(Options{ConfigDir: t.TempDir(), Keyring: store, Vault: store})
	host := &rpcTestPrompter{answers: map[string]string{
		"Storage type (gdrive, yandex, s3, webdav)": "webdav", "Connection name": "My DAV",
		"Server URL": "https://dav.example.test", "Username": "user", "Password": "private credential",
	}}
	p.prompter = host
	if err := p.RunPluginCommand(cloudFoxAddConnectionCommandID); err != nil {
		t.Fatal(err)
	}
	connections, err := p.plugin.repo.List(context.Background())
	if err != nil || len(connections) != 1 {
		t.Fatalf("connections=%v err=%v", connections, err)
	}
	values, err := p.plugin.repo.Credentials(context.Background(), connections[0])
	if err != nil || values["password"] != "private credential" {
		t.Fatalf("saved credentials missing: %v", err)
	}
	for _, request := range host.requests {
		if request.Prompt == "Password" && (!request.Password || request.Default != "") {
			t.Fatal("unsafe password prompt")
		}
	}
	host.cancel = "Connection name"
	if err := p.RunPluginCommand(cloudFoxAddConnectionCommandID); err != nil {
		t.Fatal(err)
	}
	connections, _ = p.plugin.repo.List(context.Background())
	if len(connections) != 1 {
		t.Fatal("cancelled profile was saved")
	}
	host.cancel = ""
	host.answers["Password"] = ""
	host.answers["Connection name"] = "Renamed DAV"
	if err := p.RunPluginCommand(cloudFoxEditConnectionCommandID); err != nil {
		t.Fatal(err)
	}
	connections, _ = p.plugin.repo.List(context.Background())
	if connections[0].Name != "Renamed DAV" {
		t.Fatal("profile rename failed")
	}
	values, err = p.plugin.repo.Credentials(context.Background(), connections[0])
	if err != nil || values["password"] != "private credential" {
		t.Fatal("editing blank password lost saved credential")
	}
	host.answers["Server URL"] = "https://other.example.test"
	if err := p.RunPluginCommand(cloudFoxEditConnectionCommandID); err == nil {
		t.Fatal("retargeted profile reused old credentials")
	}
}

func TestRPCCloudVaultPromptCancels(t *testing.T) {
	p := NewRPCPlugin(Options{ConfigDir: t.TempDir()})
	p.prompter = &rpcTestPrompter{cancel: "Master password"}
	_, err := p.plugin.vault.prompt.PromptMasterPassword(context.Background(), false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel err=%v", err)
	}
}

func TestRPCCloudCreatesAndBrowsesWebDAV(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(&webdav.Handler{FileSystem: webdav.Dir(dir), LockSystem: webdav.NewMemLS()})
	defer server.Close()
	p := NewRPCPlugin(Options{ConfigDir: t.TempDir(), Keyring: failingSecretStore{err: errors.New("must not save anonymous credentials")}})
	p.prompter = &rpcTestPrompter{answers: map[string]string{"Storage type (gdrive, yandex, s3, webdav)": "webdav", "Connection name": "DAV", "Server URL": server.URL, "Authentication (basic, digest, bearer, anonymous)": "anonymous"}}
	if err := p.RunPluginCommand(cloudFoxAddConnectionCommandID); err != nil {
		t.Fatal(err)
	}
	items, err := p.ReadDir(DriveName, "/DAV/")
	if err != nil || len(items) != 1 || items[0].Name != "hello.txt" {
		t.Fatalf("listing=%v err=%v", items, err)
	}
	id, size, err := p.Open(DriveName, "/DAV/hello.txt")
	if err != nil || size != 5 {
		t.Fatalf("open size=%d err=%v", size, err)
	}
	data, err := p.ReadAt(id, 5, 0)
	if err != nil || string(data) != "hello" {
		t.Fatalf("read=%q err=%v", data, err)
	}
	if err := p.CloseFile(id); err != nil {
		t.Fatal(err)
	}
	p.forgetConnection("DAV")
}

func TestRPCCloudUsesHostPortableProfile(t *testing.T) {
	dir := t.TempDir()
	p := NewRPCPlugin()
	if !p.hostProfile {
		t.Fatal("default plugin ignores the host profile")
	}
	if err := p.useProfile(f4plugin.ProfileInfo{Dir: dir, Portable: true}); err != nil {
		t.Fatal(err)
	}
	if got := p.plugin.repo.Connections.Path(); got != filepath.Join(dir, "CloudFox.json") {
		t.Fatalf("metadata path=%q", got)
	}
	if !p.plugin.portable || p.plugin.vault.Path() != filepath.Join(dir, "CloudFox.vault") {
		t.Fatal("portable vault does not follow host profile")
	}
	if _, ok := p.plugin.vault.prompt.(rpcMasterPasswordPrompter); !ok {
		t.Fatal("portable vault uses in-process prompt")
	}
	if NewRPCPlugin(Options{ConfigDir: dir}).hostProfile {
		t.Fatal("explicit injected profile unexpectedly replaced")
	}
}
