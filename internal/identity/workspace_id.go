package identity

import (
	"encoding/json"
	"errors"
	"math"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

// WorkspaceID is a workspace's integer key. It is internal: users, REST, MCP,
// CLI, and responses see only its short ID (see String and ParseWorkspaceID).
type WorkspaceID int64

// ErrInvalidWorkspaceID is returned for any short ID that is malformed or
// does not decode to a workspace key. Callers map it to the same not-found
// response as a missing workspace.
var ErrInvalidWorkspaceID = errors.New("invalid workspace id")

var workspaceCodec = mustWorkspaceCodec()

func mustWorkspaceCodec() *id.Codec {
	codec, err := id.NewCodec(id.TypeWorkspace, "")
	if err != nil {
		panic("identity: workspace short-id codec: " + err.Error())
	}
	return codec
}

// String returns the workspace's short ID.
func (w WorkspaceID) String() string {
	if w == 0 {
		return "" // no workspace
	}
	// Keys are positive, so the uint64 conversion is exact and Encode cannot fail.
	short, _ := workspaceCodec.Encode(uint64(w))
	return short
}

// MarshalJSON emits the short ID, never the integer key.
func (w WorkspaceID) MarshalJSON() ([]byte, error) {
	return json.Marshal(w.String())
}

// UnmarshalJSON accepts only a short-ID string, or null or "" for no workspace.
// Integer keys in JSON are rejected.
func (w *WorkspaceID) UnmarshalJSON(data []byte) error {
	var shortID *string
	if err := json.Unmarshal(data, &shortID); err != nil {
		return ErrInvalidWorkspaceID
	}
	if shortID == nil || *shortID == "" {
		*w = 0
		return nil
	}
	parsed, err := ParseWorkspaceID(*shortID)
	if err != nil {
		return err
	}
	*w = parsed
	return nil
}

// ParseWorkspaceID decodes a workspace short ID strictly. Integer keys are not
// accepted as input.
func ParseWorkspaceID(shortID string) (WorkspaceID, error) {
	value, err := workspaceCodec.Decode(shortID)
	if err != nil || value == 0 || value > math.MaxInt64 {
		return 0, ErrInvalidWorkspaceID
	}
	return WorkspaceID(value), nil
}

// nullableWorkspaceID stores an absent workspace as SQL NULL.
func nullableWorkspaceID(workspaceID WorkspaceID) any {
	if workspaceID == 0 {
		return nil
	}
	return int64(workspaceID)
}

// ParseWorkspaceIDOrZero is for adapters: a malformed ID becomes the zero key,
// which matches no workspace and gets the same denial as a missing one.
func ParseWorkspaceIDOrZero(shortID string) WorkspaceID {
	workspaceID, err := ParseWorkspaceID(shortID)
	if err != nil {
		return 0
	}
	return workspaceID
}
