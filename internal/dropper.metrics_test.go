package dropper

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetrics_EndpointContainsCustomMetrics(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// Make a request to prime the request counter.
	resp, err := http.Get(ts.URL + RouteHealthz)
	require.NoError(t, err)
	resp.Body.Close()

	// Prime the error counter (CounterVec requires at least one label set to appear).
	// Hit a non-existent route that triggers a 404.
	ErrorsTotal.WithLabelValues(ErrCodeInternal).Inc()

	// Check /metrics for custom metric names.
	resp, err = http.Get(ts.URL + RouteMetrics)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	bodyStr := string(body)

	assert.Contains(t, bodyStr, MetricsNamespace+"_"+MetricNameRequestsTotal,
		"should contain request counter metric")
	assert.Contains(t, bodyStr, MetricsNamespace+"_"+MetricNameUploadsTotal,
		"should contain upload counter metric")
	assert.Contains(t, bodyStr, MetricsNamespace+"_"+MetricNameUploadBytes,
		"should contain upload bytes metric")
	assert.Contains(t, bodyStr, MetricsNamespace+"_"+MetricNameErrorsTotal,
		"should contain error counter metric")
}

func TestMetrics_RequestCounter_IncrementedByHealthz(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// Hit healthz a few times.
	for range 3 {
		resp, err := http.Get(ts.URL + RouteHealthz)
		require.NoError(t, err)
		resp.Body.Close()
	}

	// Verify /metrics output contains the route label for /healthz.
	resp, err := http.Get(ts.URL + RouteMetrics)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	bodyStr := string(body)

	assert.Contains(t, bodyStr, `route="/healthz"`,
		"metrics should contain healthz route label")
	assert.Contains(t, bodyStr, `method="GET"`,
		"metrics should contain GET method label")
	assert.Contains(t, bodyStr, `status="200"`,
		"metrics should contain 200 status label")
}

func TestMetrics_ErrorCounter_IncrementedByError(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// Trigger a 400 error: POST /files/mkdir without auth returns 401/redirect,
	// but GET /files/download without path param returns 400 (needs auth).
	// Use a login-then-error approach.
	client := noRedirectClient()

	// Login first.
	loginForm := url.Values{}
	loginForm.Set(FormFieldLoginInput, cfg.Dropper.Secret)
	loginResp, err := client.PostForm(ts.URL+RouteLogin, loginForm)
	require.NoError(t, err)
	loginResp.Body.Close()

	// Get session cookie.
	cookies := loginResp.Cookies()
	require.NotEmpty(t, cookies, "should have session cookie after login")

	// Make request that triggers an error (download without path param).
	req, err := http.NewRequest(http.MethodGet, ts.URL+RouteFilesDownload, nil)
	require.NoError(t, err)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	errResp, err := client.Do(req)
	require.NoError(t, err)
	errResp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, errResp.StatusCode)

	// Check /metrics for error counter.
	metricsResp, err := http.Get(ts.URL + RouteMetrics)
	require.NoError(t, err)
	defer metricsResp.Body.Close()

	body, err := io.ReadAll(metricsResp.Body)
	require.NoError(t, err)
	bodyStr := string(body)

	assert.Contains(t, bodyStr, `error_code="bad_request"`,
		"metrics should contain bad_request error code")
}

func TestMetrics_UploadCounter_IncrementedByUpload(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := noRedirectClient()

	// Login.
	loginForm := url.Values{}
	loginForm.Set(FormFieldLoginInput, cfg.Dropper.Secret)
	loginResp, err := client.PostForm(ts.URL+RouteLogin, loginForm)
	require.NoError(t, err)
	loginResp.Body.Close()
	cookies := loginResp.Cookies()
	require.NotEmpty(t, cookies)

	// Upload a file.
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile(FormFieldFile, "test-metrics.txt")
	require.NoError(t, err)
	fileContent := "hello metrics test content"
	_, err = part.Write([]byte(fileContent))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequest(http.MethodPost, ts.URL+RouteFilesUpload+"?"+QueryParamPath+"=.", &buf)
	require.NoError(t, err)
	req.Header.Set(HeaderContentType, writer.FormDataContentType())
	for _, c := range cookies {
		req.AddCookie(c)
	}

	uploadResp, err := client.Do(req)
	require.NoError(t, err)
	uploadResp.Body.Close()
	assert.Equal(t, http.StatusOK, uploadResp.StatusCode)

	// Check /metrics for upload counters.
	metricsResp, err := http.Get(ts.URL + RouteMetrics)
	require.NoError(t, err)
	defer metricsResp.Body.Close()

	body, err := io.ReadAll(metricsResp.Body)
	require.NoError(t, err)
	bodyStr := string(body)

	// Verify upload counter appeared with a value > 0.
	assert.Contains(t, bodyStr, MetricsNamespace+"_"+MetricNameUploadsTotal,
		"metrics should contain upload counter")

	// Verify upload bytes appeared.
	assert.Contains(t, bodyStr, MetricsNamespace+"_"+MetricNameUploadBytes,
		"metrics should contain upload bytes counter")

	// Verify upload counter has a positive value (not just "0").
	uploadCounterPrefix := MetricsNamespace + "_" + MetricNameUploadsTotal + " "
	for _, line := range strings.Split(bodyStr, "\n") {
		if strings.HasPrefix(line, uploadCounterPrefix) {
			// Counter line format: "dropper_uploads_total 1"
			valueStr := strings.TrimPrefix(line, uploadCounterPrefix)
			assert.NotEqual(t, "0", strings.TrimSpace(valueStr),
				"upload counter should be greater than 0")
		}
	}
}
