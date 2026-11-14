package files

import (
	"testing"

	"github.com/aws/smithy-go"
)

func TestS3NotFoundRecognizesGenericProviderErrors(t *testing.T) {
	if !isS3NotFound(&smithy.GenericAPIError{Code: "NotFound"}) {
		t.Fatal("generic NotFound error was not normalized")
	}
}
