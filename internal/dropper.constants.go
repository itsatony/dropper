package dropper

import "time"

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
	EnvListenPort     = "DROPPER_LISTEN_PORT"
	EnvSessionTTL     = "DROPPER_SESSION_TTL"
	EnvRateLimitLogin = "DROPPER_RATE_LIMIT_LOGIN"
	EnvRootDir        = "DROPPER_ROOT_DIR"
	EnvReadonly       = "DROPPER_READONLY"
	EnvMaxUploadBytes = "DROPPER_MAX_UPLOAD_BYTES"
	EnvAllowedExts    = "DROPPER_ALLOWED_EXTENSIONS"
	EnvAuditLogPath   = "DROPPER_AUDIT_LOG_PATH"
	EnvLoggingLevel   = "DROPPER_LOGGING_LEVEL"
	EnvLoggingFormat  = "DROPPER_LOGGING_FORMAT"
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
	RouteHealthz = "/healthz"
	RouteVersion = "/version"
	RouteMetrics = "/metrics"
	RouteStatic  = "/static/*"
	RouteLogin   = "/login"
	RouteLogout  = "/logout"
	RouteRoot    = "/"
)

// --- Static file serving ---

const (
	StaticFSPrefix  = "static"
	StaticURLPrefix = "/static/"
)

// --- HTTP headers ---

const (
	HeaderContentType      = "Content-Type"
	HeaderXContentTypeOpts = "X-Content-Type-Options"
	HeaderXFrameOptions    = "X-Frame-Options"
	HeaderCSP              = "Content-Security-Policy"
)

// --- Header values ---

const (
	ContentTypeJSON = "application/json"
	ContentTypeHTML = "text/html; charset=utf-8"
	ValueNoSniff    = "nosniff"
	ValueFrameDeny  = "DENY"
	ValueCSPDefault = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'"
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
)

// --- Response error codes ---

const (
	ErrCodeInternal = "internal_error"
)

// --- Context key type for type safety ---

type ctxKey string
