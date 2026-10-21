package f4plugin

import (
	"errors"
	"io"
	"testing"

	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/vmihailenco/msgpack/v5"
)

func TestHostPromptRoundTripAndErrors(t *testing.T) {
	toHostR, toHostW := io.Pipe()
	toPluginR, toPluginW := io.Pipe()
	server := f4rpc.NewSession(toHostR, toPluginW)
	client := f4rpc.NewSession(toPluginR, toHostW)
	done := make(chan error, 2)
	go func() { done <- server.Serve() }()
	go func() { done <- client.Serve() }()
	t.Cleanup(func() { _ = toHostW.Close(); _ = toPluginW.Close(); <-done; <-done })
	request := PromptRequest{Title: "Credentials", Prompt: "Token", Password: true}
	want := PromptResponse{Text: "token", Accepted: true}
	server.Register("Host.Prompt", func(data msgpack.RawMessage) (any, error) {
		var got PromptRequest
		if err := msgpack.Unmarshal(data, &got); err != nil {
			return nil, err
		}
		if got != request {
			return nil, errors.New("prompt request changed in transit")
		}
		return want, nil
	})
	host := &Host{sess: client}
	profile := ProfileInfo{Dir: "custom-profile", Portable: true}
	server.Register("Host.GetProfile", func(msgpack.RawMessage) (any, error) { return profile, nil })
	if got, err := host.Profile(); err != nil || got != profile {
		t.Fatalf("profile=%#v err=%v", got, err)
	}
	response, err := host.Prompt(request)
	if err != nil || response != want {
		t.Fatalf("response=%#v err=%v", response, err)
	}
	server.Register("Host.Prompt", func(msgpack.RawMessage) (any, error) { return PromptResponse{}, nil })
	response, err = host.Prompt(request)
	if err != nil || response.Accepted || response.Text != "" {
		t.Fatal("cancellation was lost")
	}
	server.Register("Host.Prompt", func(msgpack.RawMessage) (any, error) { return nil, errors.New("UI blocked") })
	if _, err := host.Prompt(request); err == nil {
		t.Fatal("host error was lost")
	}
}
