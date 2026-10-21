package f4plugin

// PromptRequest asks for a single value without saving it in input history.
// Password masks the field; Default must never contain a stored secret.
type PromptRequest struct {
	Title, Prompt, Default string
	Password               bool
}

// PromptResponse distinguishes cancelling from accepting an empty value.
type PromptResponse struct {
	Text     string
	Accepted bool
}

// Prompt asks for a value, preserving cancellation and host errors. Unlike
// InputBox, it also supports transient masked credential entry.
func (h *Host) Prompt(request PromptRequest) (PromptResponse, error) {
	var response PromptResponse
	err := h.sess.Call("Host.Prompt", request, &response)
	return response, err
}
