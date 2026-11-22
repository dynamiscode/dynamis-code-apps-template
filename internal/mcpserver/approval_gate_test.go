package mcpserver

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestApprovalGateBindsConfirmationToToolAndArguments(t *testing.T) {
	input := map[string]string{"id": "one"}
	request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}}
	prompt, err := approvalGate(request, "delete", input, "Confirm deletion")
	if err != nil || prompt == nil || prompt.RequestState == "" || len(prompt.InputRequests) == 0 {
		t.Fatalf("initial approval = %+v, %v", prompt, err)
	}
	request.Params.InputResponses = mcp.InputResponseMap{
		approvalInputID: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}},
	}
	for _, tc := range []struct {
		name, state, tool string
		input             map[string]string
	}{
		{"forged state", "forged", "delete", input},
		{"different tool", prompt.RequestState, "other", input},
		{"different arguments", prompt.RequestState, "delete", map[string]string{"id": "two"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request.Params.RequestState = tc.state
			result, err := approvalGate(request, tc.tool, tc.input, "Confirm deletion")
			if err != nil || result == nil || len(result.InputRequests) == 0 {
				t.Fatalf("unbound approval = %+v, %v", result, err)
			}
		})
	}
	request.Params.RequestState = prompt.RequestState
	request.Params.InputResponses[approvalInputID] = &mcp.ElicitResult{Action: "decline"}
	if _, err := approvalGate(request, "delete", input, "Confirm deletion"); err == nil {
		t.Fatal("declined approval accepted")
	}
	request.Params.InputResponses[approvalInputID] = &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	if result, err := approvalGate(request, "delete", input, "Confirm deletion"); result != nil || err != nil {
		t.Fatalf("accepted approval = %+v, %v", result, err)
	}
}
