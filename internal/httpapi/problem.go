package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail"`
	Instance  string `json:"instance"`
	Code      string `json:"code"`
	RequestID string `json:"requestId"`
}

func writeProblem(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	code string,
	detail string,
) {
	requestID := requestIDFrom(request.Context())
	writer.Header().Set("Content-Type", "application/problem+json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(problem{
		Type: "urn:dynamis-code:problem:" + code, Title: http.StatusText(status),
		Status: status, Detail: detail,
		Instance: "urn:request:" + requestID, Code: code, RequestID: requestID,
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func internalProblem(writer http.ResponseWriter, request *http.Request) {
	writeProblem(
		writer, request, http.StatusInternalServerError, "internal-error",
		"The request could not be completed.",
	)
}

func methodProblem(writer http.ResponseWriter, request *http.Request) {
	writeProblem(
		writer, request, http.StatusMethodNotAllowed, "method-not-allowed",
		"The method is not supported for this resource.",
	)
}

func notFoundProblem(writer http.ResponseWriter, request *http.Request) {
	writeProblem(
		writer, request, http.StatusNotFound, "not-found",
		"The requested resource was not found.",
	)
}

func etag(version int64) string {
	return fmt.Sprintf(`"v%d"`, version)
}
