package dropper

import (
	"os"
	"time"
)

// --- Service identity ---

const (
	ServiceName = "dropper"
)

// --- Environment variable prefix ---

const (
	EnvPrefix = "DROPPER"
)

// --- Config keys (viper binding) ---

const (
	ConfigKeyListenPort        = "dropper.listen_port"
	ConfigKeySessionTTL        = "dropper.session_ttl"
	ConfigKeyRateLimitLogin    = "dropper.rate_limit_login"
	ConfigKeyRootDir           = "dropper.root_dir"
	ConfigKeyReadonly          = "dropper.readonly"
	ConfigKeyMaxUploadBytes    = "dropper.max_upload_bytes"
	ConfigKeyAllowedExtensions = "dropper.allowed_extensions"
	ConfigKeyAuditLogPath      = "dropper.audit_log_path"
	ConfigKeyLoggingLevel      = "dropper.logging.level"
	ConfigKeyLoggingFormat     = "dropper.logging.format"
	ConfigKeyLoggingOutput     = "dropper.logging.output"
	ConfigKeyLoggingNoLogPaths = "dropper.logging.no_log_paths"
)

// ConfigKeySecret is the viper key for the auth secret.
const ConfigKeySecret = "dropper." + "secret"

// --- Environment variable names ---

const (
	EnvListenPort        = "DROPPER_LISTEN_PORT"
	EnvSessionTTL        = "DROPPER_SESSION_TTL"
	EnvRateLimitLogin    = "DROPPER_RATE_LIMIT_LOGIN"
	EnvRootDir           = "DROPPER_ROOT_DIR"
	EnvReadonly          = "DROPPER_READONLY"
	EnvMaxUploadBytes    = "DROPPER_MAX_UPLOAD_BYTES"
	EnvAllowedExts       = "DROPPER_ALLOWED_EXTENSIONS"
	EnvAuditLogPath      = "DROPPER_AUDIT_LOG_PATH"
	EnvLoggingLevel      = "DROPPER_LOGGING_LEVEL"
	EnvLoggingFormat     = "DROPPER_LOGGING_FORMAT"
	EnvLoggingOutput     = "DROPPER_LOGGING_OUTPUT"
	EnvLoggingNoLogPaths = "DROPPER_LOGGING_NO_LOG_PATHS"
)

// EnvSecret is the environment variable name for the auth secret.
const EnvSecret = EnvPrefix + "_SECRET"

// --- Config file search paths ---

const (
	DefaultConfigName     = "dropper"
	ConfigFileType        = "yaml"
	ConfigSearchPathLocal = "./configs"
	ConfigSearchPathEtc   = "/etc/dropper"
)

// --- CLI flags ---

const (
	FlagConfigName  = "config"
	FlagConfigUsage = "path to config file"
)

// --- Default config values ---

const (
	DefaultListenPort     = 8080
	DefaultSessionTTL     = "24h"
	DefaultRateLimitLogin = 5
	DefaultRootDir        = "./data"
	DefaultMaxUploadBytes = 104857600 // 100 MB
	DefaultAuditLogPath   = "dropper_audit.log"
	DefaultLogLevel       = "info"
	DefaultLogFormat      = "json"
	DefaultLogOutput      = "stdout"
	DefaultReadonly       = false
)

// --- Server timeouts ---

const (
	DefaultReadTimeout     = 30 * time.Second
	DefaultWriteTimeout    = 60 * time.Second
	DefaultIdleTimeout     = 120 * time.Second
	DefaultShutdownTimeout = 15 * time.Second
)

// --- Route paths ---

const (
	RouteHealthz       = "/healthz"
	RouteVersion       = "/version"
	RouteMetrics       = "/metrics"
	RouteStatic        = "/static/*"
	RouteLogin         = "/login"
	RouteLogout        = "/logout"
	RouteRoot          = "/"
	RouteFiles         = "/files"
	RouteFilesDownload = "/files/download"
	RouteFilesUpload   = "/files/upload"
	RouteFilesMkdir    = "/files/mkdir"
)

// --- Query parameter names ---

const (
	QueryParamPath      = "path"
	QueryParamSortBy    = "sort"
	QueryParamSortOrder = "order"
	QueryParamName      = "name"
	QueryParamClipboard = "clipboard"
)

// --- Default query values ---

const (
	DefaultBrowsePath = "."
)

// --- Static file serving ---

const (
	StaticFSPrefix  = "static"
	StaticURLPrefix = "/static/"
)

// --- HTTP headers ---

const (
	HeaderContentType        = "Content-Type"
	HeaderContentDisposition = "Content-Disposition"
	HeaderXContentTypeOpts   = "X-Content-Type-Options"
	HeaderXFrameOptions      = "X-Frame-Options"
	HeaderCSP                = "Content-Security-Policy"
)

// --- Header values ---

const (
	ContentTypeJSON          = "application/json"
	ContentTypeHTML          = "text/html; charset=utf-8"
	ValueNoSniff             = "nosniff"
	ValueFrameDeny           = "DENY"
	ValueCSPDefault          = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'"
	ContentDispositionFormat = `attachment; filename="%s"`
	QueryParamClipboardTrue  = "true"
)

// --- Log messages ---

const (
	LogMsgStarting         = "starting dropper"
	LogMsgServerListening  = "http server listening"
	LogMsgShutdownSignal   = "received shutdown signal"
	LogMsgShutdownStarted  = "graceful shutdown started"
	LogMsgShutdownComplete = "dropper shutdown complete"
	LogMsgConfigLoaded     = "configuration loaded"
)

// --- Log field names ---

const (
	LogFieldService   = "service"
	LogFieldVersion   = "version"
	LogFieldAddr      = "addr"
	LogFieldSignal    = "signal"
	LogFieldError     = "error"
	LogFieldPort      = "port"
	LogFieldRootDir   = "root_dir"
	LogFieldReadonly  = "readonly"
	LogFieldComponent = "component"
)

// --- Log format values ---

const (
	LogFormatJSON    = "json"
	LogFormatConsole = "console"
)

// --- Log output values ---

const (
	LogOutputStdout = "stdout"
	LogOutputStderr = "stderr"
)

// --- Log level values ---

const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

// --- Health check ---

const (
	HealthStatusOK = "ok"
	DiskPercent100 = 100.0
)

// --- Fatal output format ---

const (
	FatalFormat = "fatal: %v\n"
)

// --- Error messages ---

const (
	ErrMsgConfigLoad       = "failed to load configuration"
	ErrMsgConfigValidation = "configuration validation failed"
	ErrMsgServerStart      = "failed to start http server"
	ErrMsgVersionInit      = "failed to initialize version"
	ErrMsgStaticFSSub      = "failed to create static sub-fs"
	ErrMsgJSONEncode       = "failed to encode json response"
	ErrMsgDiskUsage        = "failed to get disk usage"
	ErrMsgShutdownError    = "graceful shutdown error"
)

// --- Response error codes ---

const (
	ErrCodeInternal = "internal_error"
)

// --- Path limits ---

const (
	MaxPathLength = 4096
)

// --- Filesystem error messages ---

const (
	ErrMsgPathTooLong     = "request path exceeds maximum length"
	ErrMsgPathTraversal   = "path traversal denied"
	ErrMsgPathResolution  = "failed to resolve path"
	ErrMsgInvalidFilename = "invalid filename"
	ErrMsgExtNotAllowed   = "file extension not allowed"
	ErrMsgReadonlyMode    = "operation denied in readonly mode"
	ErrMsgListDir         = "failed to list directory"
	ErrMsgCreateDir       = "failed to create directory"
	ErrMsgWriteFile       = "failed to write file"
	ErrMsgTempFile        = "failed to create temp file"
	ErrMsgRenameFile      = "failed to rename file into place"
	ErrMsgNotFound        = "path not found"
	ErrMsgNotDirectory    = "path is not a directory"
	ErrMsgNotFile         = "path is not a file"
	ErrMsgFileTooLarge    = "file exceeds maximum upload size"
	ErrMsgFileExists      = "file already exists after collision retries"
	ErrMsgMultipartParse  = "failed to parse multipart form"
	ErrMsgNoFilesUploaded = "no files in upload request"
	ErrMsgFileStat        = "failed to stat file"
	ErrMsgBodyTooLarge    = "request body exceeds maximum size"
	ErrMsgMissingParam    = "required parameter missing"
	ErrMsgInternal        = "internal server error"
)

// --- Config validation error messages ---

const (
	ErrMsgRootDirNotExist     = "root_dir does not exist or is not a directory"
	ErrMsgExtMissingDot       = "allowed_extensions entries must start with '.'"
	ErrMsgAuditLogParentNoDir = "audit_log_path parent directory does not exist"
)

// --- Filesystem error codes ---

const (
	ErrCodePathTooLong     = "path_too_long"
	ErrCodeForbidden       = "forbidden"
	ErrCodeBadRequest      = "bad_request"
	ErrCodeNotFound        = "not_found"
	ErrCodeReadonly        = "readonly"
	ErrCodeExtNotAllowed   = "extension_not_allowed"
	ErrCodeFileTooLarge    = "file_too_large"
	ErrCodePayloadTooLarge = "payload_too_large"
)

// --- Sort fields ---

const (
	SortByName       = "name"
	SortByDate       = "date"
	SortBySize       = "size"
	SortOrderAsc     = "asc"
	SortOrderDesc    = "desc"
	DefaultSortField = SortByName
	DefaultSortOrder = SortOrderAsc
)

// --- Filename sanitization ---

const (
	FilenameSanitizePattern = `[^a-zA-Z0-9_.\-]`
	FilenameSanitizeReplace = '_'
	FilenameMaxLength       = 255
	FilenameFallback        = "_"
	ClipboardFilenamePrefix = "clipboard"
	ClipboardFilenameExt    = ".png"
)

// --- Timestamp format for collision resolution ---

const (
	CollisionTimestampFormat = "20060102-150405"
	CollisionSeparator       = "_"
	CollisionMaxRetries      = 10
)

// --- File permissions ---

const (
	DirPermissions  os.FileMode = 0755
	FilePermissions os.FileMode = 0644
)

// --- Temp file pattern ---

const (
	TempFilePattern = "dropper-upload-*"
)

// --- Multipart upload ---

const (
	MaxMultipartMemory int64 = 32 << 20 // 32 MB in-memory before disk buffering
)

// --- File size formatting ---

const (
	SizeUnitB  = "B"
	SizeUnitKB = "KB"
	SizeUnitMB = "MB"
	SizeUnitGB = "GB"
	SizeUnitTB = "TB"
)

const (
	SizeKB int64 = 1024
	SizeMB int64 = 1024 * 1024
	SizeGB int64 = 1024 * 1024 * 1024
	SizeTB int64 = 1024 * 1024 * 1024 * 1024
)

// --- File size format strings ---

const (
	SizeFormatBytes   = "%d %s"
	SizeFormatDecimal = "%.1f %s"
)

// --- Request logging ---

const (
	LogMsgRequestCompleted = "request completed"
)

// --- Request logging field names ---

const (
	LogFieldMethod   = "method"
	LogFieldURLPath  = "url_path"
	LogFieldStatus   = "status"
	LogFieldDuration = "duration_ms"
)

// --- Log messages (filesystem) ---

const (
	LogMsgPathDenied        = "path traversal attempt denied"
	LogMsgFileWritten       = "file written"
	LogMsgDirCreated        = "directory created"
	LogMsgCollisionResolved = "filename collision resolved"
	LogMsgExtRejected       = "extension rejected"
)

// --- Log messages (file handlers) ---

const (
	LogMsgUploadSuccess       = "file uploaded"
	LogMsgUploadFailed        = "upload failed"
	LogMsgUploadBatchComplete = "upload batch complete"
	LogMsgDownloadServed      = "file download served"
	LogMsgMkdirHandler        = "directory created via handler"
	LogMsgMkdirFailed         = "mkdir failed"
	LogMsgPasteUpload         = "clipboard paste uploaded"
	LogMsgMultipartCleanup    = "failed to remove multipart temp files"
	LogMsgFileHandleClose     = "failed to close uploaded file handle"
)

// --- Log field names (filesystem) ---

const (
	LogFieldPath         = "path"
	LogFieldFilename     = "filename"
	LogFieldOriginalName = "original_name"
	LogFieldResolvedName = "resolved_name"
	LogFieldSize         = "size"
	LogFieldExtension    = "extension"
	LogFieldUploadCount  = "upload_count"
	LogFieldFailCount    = "fail_count"
)

// --- Session / Auth ---

const (
	SessionCookieName      = "dropper_session"
	SessionTokenBytes      = 32 // 32 bytes -> 64-char hex string
	SessionCleanupInterval = 5 * time.Minute
	CookiePath             = "/"
	CookieDeleteMaxAge     = -1
)

// --- Rate limiting ---

const (
	RateLimitWindow = 1 * time.Minute
)

// --- Toast types ---

const (
	ToastSuccess = "success"
	ToastError   = "error"
	ToastInfo    = "info"
)

// --- Breadcrumb ---

const (
	BreadcrumbRootLabel = "Home"
)

// --- Display formats ---

const (
	ModTimeDisplayFormat = "2006-01-02 15:04"
)

// --- Auth error codes ---

const (
	ErrCodeUnauthorized = "unauthorized"
	ErrCodeTooManyReqs  = "too_many_requests"
)

// --- Auth error messages ---

const (
	ErrMsgInvalidCredential = "invalid secret"
	ErrMsgSessionNotFound   = "session not found or expired"
	ErrMsgRateLimitExceeded = "too many login attempts, try again later"
	ErrMsgTokenGeneration   = "failed to generate session token"
	ErrMsgTemplateRender    = "failed to render template"
	ErrMsgTemplateParse     = "failed to parse templates"
)

// --- File browsing log messages ---

const (
	LogMsgBrowseDenied = "directory browse denied"
)

// --- File browsing error messages ---

const (
	ErrMsgBrowsePath  = "invalid browse path"
	ErrMsgTemplateSet = "failed to create template set"
)

// --- File browsing log fields ---

const (
	LogFieldBrowsePath = "browse_path"
)

// --- Auth log messages ---

const (
	LogMsgLoginSuccess      = "login successful"
	LogMsgLoginFailed       = "login failed"
	LogMsgLogout            = "session logged out"
	LogMsgSessionCreated    = "session created"
	LogMsgSessionExpired    = "session expired"
	LogMsgSessionCleanup    = "expired sessions cleaned"
	LogMsgRateLimited       = "login rate limited"
	LogMsgAuthMiddleware    = "auth check failed"
	LogMsgSessionStoreStart = "session cleanup goroutine started"
	LogMsgSessionStoreStop  = "session cleanup goroutine stopped"
)

// --- Auth log field names ---

const (
	LogFieldIP        = "ip"
	LogFieldSessionID = "session_id"
	LogFieldExpired   = "expired_count"
)

// --- Template names ---

const (
	TemplateBaseDir       = "templates"
	TemplatePartialsDir   = "partials"
	TemplateComponentsDir = "components"
	TemplateLayout        = "layout.html"
	TemplateLogin         = "login.html"
	TemplateMain          = "main.html"
	TemplateBreadcrumbs   = "breadcrumbs.html"
	TemplateFilelist      = "filelist.html"
	TemplateDropzone      = "dropzone.html"
	TemplateBookmarks     = "bookmarks.html"
	TemplateToast         = "toast.html"
	TemplatePreview       = "preview.html"
)

// --- Template page keys (for TemplateSet map) ---

const (
	PageLogin = "login"
	PageMain  = "main"
)

// --- Template block names ---

const (
	BlockBreadcrumbs = "breadcrumbs"
	BlockFilelist    = "filelist"
	BlockDropzone    = "dropzone"
	BlockBookmarks   = "bookmarks"
	BlockToast       = "toast"
	BlockPreview     = "preview"
)

// --- HTTP headers (auth) ---

const (
	HeaderAccept   = "Accept"
	HeaderLocation = "Location"
)

// --- HTMX headers ---

const (
	HeaderHXRequest = "HX-Request"
	HeaderHXTarget  = "HX-Target"
	HeaderHXTrigger = "HX-Trigger"
	HeaderHXSwapOOB = "HX-Swap-Oob"
)

// --- HTMX header values ---

const (
	HXRequestTrue = "true"
)

// --- Content types (auth) ---

const (
	ContentTypeForm = "application/x-www-form-urlencoded"
)

// --- Form field names ---

const (
	FormFieldLoginInput = "secret"
	FormFieldFile       = "file"
)

// --- Session token format ---

const (
	SessionTokenLogPrefixLen = 8
)

// --- Audit actions ---

const (
	AuditActionUpload   = "upload"
	AuditActionDownload = "download"
	AuditActionMkdir    = "mkdir"
)

// --- Audit timestamp format ---

const (
	AuditTimestampFormat = "2006-01-02T15:04:05.999999999Z07:00" // time.RFC3339Nano
)

// --- Audit log messages ---

const (
	LogMsgAuditStarted  = "audit logger started"
	LogMsgAuditClosed   = "audit logger closed"
	LogMsgAuditDisabled = "audit logger disabled (no path configured)"
	LogMsgAuditWriteErr = "failed to write audit entry"
	LogMsgAuditReopened = "audit log file reopened"
)

// --- Audit error messages ---

const (
	ErrMsgAuditInit  = "failed to initialize audit logger"
	ErrMsgAuditOpen  = "failed to open audit log file"
	ErrMsgAuditClose = "failed to close audit log file"
)

// --- Audit log field names ---

const (
	LogFieldAuditPath = "audit_path"
)

// --- Prometheus metrics ---

const (
	MetricsNamespace = "dropper"
)

// --- Metric names ---

const (
	MetricNameRequestsTotal = "http_requests_total"
	MetricNameUploadsTotal  = "uploads_total"
	MetricNameUploadBytes   = "upload_bytes_total"
	MetricNameErrorsTotal   = "errors_total"
)

// --- Metric labels ---

const (
	MetricLabelMethod    = "method"
	MetricLabelRoute     = "route"
	MetricLabelStatus    = "status"
	MetricLabelErrorCode = "error_code"
)

// --- Metric help strings ---

const (
	MetricHelpRequests    = "Total number of HTTP requests"
	MetricHelpUploads     = "Total number of successful file uploads"
	MetricHelpUploadBytes = "Total bytes uploaded successfully"
	MetricHelpErrors      = "Total number of error responses"
)

// --- Metric route label fallback ---

const (
	MetricRouteUnknown = "unknown"
)
