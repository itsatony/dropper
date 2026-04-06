package dropper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	data := map[string]string{"key": "value"}

	RespondJSON(rec, http.StatusCreated, data)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, ContentTypeJSON, rec.Header().Get(HeaderContentType))

	var body map[string]string
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "value", body["key"])
}

func TestRespondJSON_NilData(t *testing.T) {
	rec := httptest.NewRecorder()

	RespondJSON(rec, http.StatusNoContent, nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String())
}

func TestRespondOK(t *testing.T) {
	rec := httptest.NewRecorder()
	data := map[string]int{"count": 42}

	RespondOK(rec, data)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]int
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, 42, body["count"])
}

func TestRespondError(t *testing.T) {
	rec := httptest.NewRecorder()

	RespondError(rec, http.StatusBadRequest, "bad_input", "invalid field")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, ContentTypeJSON, rec.Header().Get(HeaderContentType))

	var body ErrorBody
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "bad_input", body.Code)
	assert.Equal(t, "invalid field", body.Message)
}
