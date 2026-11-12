package files

import (
	"encoding/json"
	"errors"
	"math"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

// FileID is a file's integer key. It is internal: users, REST, and web see
// only its short ID (see String and ParseFileID).
type FileID int64

// ErrInvalidFileID is returned for any short ID that is malformed or does not
// decode to a file key. Callers map it to the same not-found response as a
// missing file.
var ErrInvalidFileID = errors.New("invalid file id")

var fileCodec = mustFileCodec()

func mustFileCodec() *id.Codec {
	codec, err := id.NewCodec(id.TypeFile, "")
	if err != nil {
		panic("files: file short-id codec: " + err.Error())
	}
	return codec
}

// String returns the file's short ID, or "" for the zero key.
func (f FileID) String() string {
	if f == 0 {
		return ""
	}
	// Keys are positive, so the uint64 conversion is exact and Encode cannot fail.
	short, _ := fileCodec.Encode(uint64(f))
	return short
}

// MarshalJSON emits the short ID, never the integer key.
func (f FileID) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.String())
}

// UnmarshalJSON accepts only a short-ID string. Integer keys in JSON are rejected.
func (f *FileID) UnmarshalJSON(data []byte) error {
	var shortID string
	if err := json.Unmarshal(data, &shortID); err != nil {
		return ErrInvalidFileID
	}
	parsed, err := ParseFileID(shortID)
	if err != nil {
		return err
	}
	*f = parsed
	return nil
}

// ParseFileID decodes a file short ID strictly. Integer keys are not accepted.
func ParseFileID(shortID string) (FileID, error) {
	value, err := fileCodec.Decode(shortID)
	if err != nil || value == 0 || value > math.MaxInt64 {
		return 0, ErrInvalidFileID
	}
	return FileID(value), nil
}

// ParseFileIDOrZero is for adapters: a malformed ID becomes the zero key,
// which matches no file and gets the same not-found as a missing one.
func ParseFileIDOrZero(shortID string) FileID {
	fileID, err := ParseFileID(shortID)
	if err != nil {
		return 0
	}
	return fileID
}
