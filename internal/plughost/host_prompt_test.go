package plughost

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/sdk/f4plugin"
	"github.com/unxed/vtui"
)

func TestRPCPromptMasksSecretsAndRepliesOnCancel(t *testing.T) {
	old := vtui.FrameManager
	fm := vtui.NewFrameManager()
	fm.Init(vtui.NewSilentScreenBuf())
	vtui.FrameManager = fm
	t.Cleanup(func() { fm.Shutdown(); vtui.FrameManager = old })
	for _, accept := range []bool{false, true} {
		result := make(chan f4plugin.PromptResponse, 1)
		dialog, edit := showRPCPrompt(f4plugin.PromptRequest{Title: "Credentials", Prompt: "Password", Password: true}, result)
		if !edit.PasswordMode {
			t.Fatal("credential edit is not masked")
		}
		edit.SetText("secret")
		if accept {
			for _, child := range dialog.GetChildren() {
				if button, ok := child.(*vtui.Button); ok && button.IsDefault {
					button.OnClick()
					break
				}
			}
		} else {
			dialog.OnResult(-1)
		}
		select {
		case response := <-result:
			if response.Accepted != accept || (accept && response.Text != "secret") || (!accept && response.Text != "") {
				t.Fatalf("unexpected reply: %#v", response)
			}
		default:
			t.Fatal("prompt did not reply")
		}
		if edit.GetText() != "" {
			t.Fatal("secret retained after dismissal")
		}
	}
}

func TestRPCHostReturnsItsResolvedProfile(t *testing.T) {
	methods := newHostMethods(newLuaTestHostAPI(), &guardTestTransport{}, "test", nil)
	value, err := methods["Host.GetProfile"](nil)
	profile, ok := value.(f4plugin.ProfileInfo)
	if err != nil || !ok || profile.Dir != config.GetF4ConfigDir() || profile.Portable != config.IsPortableProfile() {
		t.Fatalf("profile=%#v err=%v", value, err)
	}
}
