package plughost

import (
	"errors"

	"github.com/unxed/f4/sdk/f4plugin"
	"github.com/unxed/vtui"
	"github.com/vmihailenco/msgpack/v5"
)

func rpcPrompt(back PluginTransport, data msgpack.RawMessage) (any, error) {
	if uiBlockedFor(back) {
		return nil, errUIBlocked
	}
	var request f4plugin.PromptRequest
	if err := msgpack.Unmarshal(data, &request); err != nil {
		return nil, err
	}
	if vtui.FrameManager == nil {
		return nil, errors.New("plugin prompt requires an active UI")
	}
	result := make(chan f4plugin.PromptResponse, 1)
	vtui.FrameManager.PostTask(func() { showRPCPrompt(request, result) })
	return <-result, nil
}

func showRPCPrompt(request f4plugin.PromptRequest, result chan<- f4plugin.PromptResponse) (*vtui.Window, *vtui.Edit) {
	lines := vtui.WrapText(request.Prompt, 58)
	dialog := vtui.NewCenteredDialog(64, 8+len(lines), request.Title)
	dialog.ShowClose = true
	x, y := dialog.X1+2, dialog.Y1+2
	edit := vtui.NewEdit(x, y+len(lines), 58, request.Default)
	if request.Password {
		edit = vtui.NewPasswordEdit(x, y+len(lines), 58, "")
	}
	for i, line := range lines {
		dialog.AddItem(vtui.NewLabel(x, y+i, line, edit))
	}
	dialog.AddItem(edit)
	accept := vtui.NewButton(x+16, dialog.Y2-2, vtui.Msg("vtui.Ok"))
	accept.IsDefault = true
	cancel := vtui.NewButton(x+30, dialog.Y2-2, vtui.Msg("vtui.Cancel"))
	dialog.AddItem(accept)
	dialog.AddItem(cancel)
	finished := false
	finish := func(accepted bool) {
		if finished {
			return
		}
		finished = true
		response := f4plugin.PromptResponse{Accepted: accepted}
		if accepted {
			response.Text = edit.GetText()
		}
		edit.SetText("")
		result <- response
	}
	dialog.OnResult = func(int) { finish(false) }
	accept.OnClick = func() { finish(true); dialog.Close() }
	cancel.OnClick = func() { dialog.Close() }
	vtui.FrameManager.Push(dialog)
	return dialog, edit
}
