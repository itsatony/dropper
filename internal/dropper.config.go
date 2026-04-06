package dropper

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

// Config is the top-level configuration structure.
type Config struct {
	Dropper DropperConfig `mapstructure:"dropper" validate:"required"`
}

// DropperConfig contains all dropper-specific configuration.
type DropperConfig struct {
	ListenPort        int           `mapstructure:"listen_port" validate:"required,min=1,max=65535"`
	Secret            string        `mapstructure:"secret" validate:"required,min=8"`
	SessionTTL        string        `mapstructure:"session_ttl" validate:"required"`
	RateLimitLogin    int           `mapstructure:"rate_limit_login" validate:"required,min=1,max=100"`
	RootDir           string        `mapstructure:"root_dir" validate:"required"`
	Readonly          bool          `mapstructure:"readonly"`
	MaxUploadBytes    int64         `mapstructure:"max_upload_bytes" validate:"required,min=1024"`
	AllowedExtensions []string      `mapstructure:"allowed_extensions"`
	AuditLogPath      string        `mapstructure:"audit_log_path" validate:"required"`
	Logging           LoggingConfig `mapstructure:"logging" validate:"required"`
}

// LoggingConfig contains logging configuration.
type LoggingConfig struct {
	Level      string   `mapstructure:"level" validate:"required,oneof=debug info warn error"`
	Format     string   `mapstructure:"format" validate:"required,oneof=json console"`
	Output     string   `mapstructure:"output" validate:"required,oneof=stdout stderr"`
	NoLogPaths []string `mapstructure:"no_log_paths"`
}

// SessionTTLDuration parses the SessionTTL string into a time.Duration.
func (c *DropperConfig) SessionTTLDuration() (time.Duration, error) {
	return time.ParseDuration(c.SessionTTL)
}

// LoadConfig loads configuration from file + environment variable overrides.
// If path is empty, searches standard locations (./configs/, /etc/dropper/).
func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigType(ConfigFileType)

	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName(DefaultConfigName)
		v.AddConfigPath(ConfigSearchPathLocal)
		v.AddConfigPath(ConfigSearchPathEtc)
	}

	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	bindEnvVars(v)
	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("%s: %w", ErrMsgConfigLoad, err)
		}
		// ConfigFileNotFoundError is acceptable — config can come entirely from env vars.
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", ErrMsgConfigLoad, err)
	}

	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", ErrMsgConfigValidation, err)
	}

	if _, err := cfg.Dropper.SessionTTLDuration(); err != nil {
		return nil, fmt.Errorf("%s: session_ttl is not a valid duration: %w",
			ErrMsgConfigValidation, err)
	}

	if err := validateRootDir(cfg.Dropper.RootDir); err != nil {
		return nil, fmt.Errorf("%s: %w", ErrMsgConfigValidation, err)
	}

	if err := validateAllowedExtensions(cfg.Dropper.AllowedExtensions); err != nil {
		return nil, fmt.Errorf("%s: %w", ErrMsgConfigValidation, err)
	}

	if err := validateAuditLogPath(cfg.Dropper.AuditLogPath); err != nil {
		return nil, fmt.Errorf("%s: %w", ErrMsgConfigValidation, err)
	}

	return cfg, nil
}

// validateRootDir checks that the configured root directory exists and is a directory.
func validateRootDir(rootDir string) error {
	info, err := os.Stat(rootDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%s: %s", ErrMsgRootDirNotExist, rootDir)
	}
	return nil
}

// validateAllowedExtensions checks that each entry starts with a dot.
func validateAllowedExtensions(exts []string) error {
	for _, ext := range exts {
		if !strings.HasPrefix(ext, ".") {
			return fmt.Errorf("%s: %q", ErrMsgExtMissingDot, ext)
		}
	}
	return nil
}

// validateAuditLogPath checks that the parent directory of the audit log path exists.
func validateAuditLogPath(auditPath string) error {
	if auditPath == "" {
		return nil
	}
	parentDir := filepath.Dir(auditPath)
	info, err := os.Stat(parentDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%s: %s", ErrMsgAuditLogParentNoDir, parentDir)
	}
	return nil
}

// bindEnvVars explicitly binds environment variable names to nested viper keys.
// Required because viper's AutomaticEnv does not reliably map prefixed env vars
// to nested YAML keys.
func bindEnvVars(v *viper.Viper) {
	_ = v.BindEnv(ConfigKeyListenPort, EnvListenPort)
	_ = v.BindEnv(ConfigKeySecret, EnvSecret)
	_ = v.BindEnv(ConfigKeySessionTTL, EnvSessionTTL)
	_ = v.BindEnv(ConfigKeyRateLimitLogin, EnvRateLimitLogin)
	_ = v.BindEnv(ConfigKeyRootDir, EnvRootDir)
	_ = v.BindEnv(ConfigKeyReadonly, EnvReadonly)
	_ = v.BindEnv(ConfigKeyMaxUploadBytes, EnvMaxUploadBytes)
	_ = v.BindEnv(ConfigKeyAllowedExtensions, EnvAllowedExts)
	_ = v.BindEnv(ConfigKeyAuditLogPath, EnvAuditLogPath)
	_ = v.BindEnv(ConfigKeyLoggingLevel, EnvLoggingLevel)
	_ = v.BindEnv(ConfigKeyLoggingFormat, EnvLoggingFormat)
	_ = v.BindEnv(ConfigKeyLoggingOutput, EnvLoggingOutput)
	_ = v.BindEnv(ConfigKeyLoggingNoLogPaths, EnvLoggingNoLogPaths)
}

// setDefaults sets default values for all config keys.
func setDefaults(v *viper.Viper) {
	v.SetDefault(ConfigKeyListenPort, DefaultListenPort)
	v.SetDefault(ConfigKeySessionTTL, DefaultSessionTTL)
	v.SetDefault(ConfigKeyRateLimitLogin, DefaultRateLimitLogin)
	v.SetDefault(ConfigKeyRootDir, DefaultRootDir)
	v.SetDefault(ConfigKeyReadonly, DefaultReadonly)
	v.SetDefault(ConfigKeyMaxUploadBytes, DefaultMaxUploadBytes)
	v.SetDefault(ConfigKeyAuditLogPath, DefaultAuditLogPath)
	v.SetDefault(ConfigKeyLoggingLevel, DefaultLogLevel)
	v.SetDefault(ConfigKeyLoggingFormat, DefaultLogFormat)
	v.SetDefault(ConfigKeyLoggingOutput, DefaultLogOutput)
}
