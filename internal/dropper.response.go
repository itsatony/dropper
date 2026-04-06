package dropper

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// RespondJSON writes a JSON response with the given status code and data.
func RespondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set(HeaderContentType, ContentTypeJSON)
	w.WriteHeader(status)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			slog.Error(ErrMsgJSONEncode, LogFieldError, err)
		}
	}
}

// RespondOK writes a 200 JSON response with the given data.
func RespondOK(w http.ResponseWriter, data any) {
	RespondJSON(w, http.StatusOK, data)
}

// ErrorBody is the standard error response body.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RespondError writes a JSON error response.
func RespondError(w http.ResponseWriter, status int, code string, message string) {
	RespondJSON(w, status, ErrorBody{
		Code:    code,
		Message: message,
	})
}

// UploadResult describes the outcome of a single file upload.
type UploadResult struct {
	OriginalName string `json:"original_name"`
	FinalName    string `json:"final_name"`
	Size         int64  `json:"size"`
	Error        string `json:"error,omitempty"`
}

// UploadResponse is the response body for POST /files/upload.
type UploadResponse struct {
	Results  []UploadResult `json:"results"`
	Uploaded int            `json:"uploaded"`
	Failed   int            `json:"failed"`
}

// MkdirResponse is the response body for POST /files/mkdir.
type MkdirResponse struct {
	Name string `json:"name"`
	Path string `json:"path"`
}
