package bootstrap

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Env struct {
	AppEnv          string `mapstructure:"APP_ENV"`
	HTTPPort        int    `mapstructure:"HTTP_PORT"`
	APIToken        string `mapstructure:"API_TOKEN"`
	SQLitePath      string `mapstructure:"SQLITE_PATH"`
	AnthropicAPIKey string `mapstructure:"ANTHROPIC_API_KEY"`
	AnthropicModel  string `mapstructure:"ANTHROPIC_MODEL"`
}

func LoadEnv() (*Env, error) {
	v := viper.New()
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	_ = v.ReadInConfig() // .env is optional; ignore missing
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// defaults
	v.SetDefault("APP_ENV", "dev")
	v.SetDefault("HTTP_PORT", 8080)
	v.SetDefault("SQLITE_PATH", "./data/app.db")
	v.SetDefault("ANTHROPIC_MODEL", "claude-haiku-4-5-20251001")

	var env Env
	if err := v.Unmarshal(&env); err != nil {
		return nil, fmt.Errorf("unmarshal env: %w", err)
	}

	if env.APIToken == "" {
		return nil, fmt.Errorf("API_TOKEN is required")
	}
	if env.SQLitePath == "" {
		return nil, fmt.Errorf("SQLITE_PATH is required")
	}
	return &env, nil
}
