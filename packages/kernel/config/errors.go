package config

type ConfigError struct {
	msg string
}

func NewConfigError(msg string) *ConfigError {
	return &ConfigError{msg: msg}
}

func (e *ConfigError) Error() string {
	return "config error: " + e.msg
}
