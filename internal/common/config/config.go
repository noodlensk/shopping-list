package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

type Config struct {
	Telegram struct {
		Token        string
		AllowedUsers []int64
	}
	Storage struct {
		Path string
	}
}

func Parse(paths ...string) (*Config, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	vp := viper.New()

	for _, path := range paths {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("file %q does not exist", path)
		}

		vp.SetConfigFile(path)

		if err := vp.MergeInConfig(); err != nil {
			return nil, errors.Wrap(err, "read config")
		}
	}

	cfg := &Config{}

	if err := vp.Unmarshal(cfg); err != nil {
		return nil, errors.Wrap(err, "unmarshal config to obj")
	}

	var validationErrs []string

	if cfg.Telegram.Token == "" {
		validationErrs = append(validationErrs, "Telegram.Token should not be empty")
	}

	if len(cfg.Telegram.AllowedUsers) == 0 {
		validationErrs = append(validationErrs, "Telegram.AllowedUsers should not be empty")
	}

	if cfg.Storage.Path == "" {
		validationErrs = append(validationErrs, "Storage.Path should not be empty")
	}

	if len(validationErrs) > 0 {
		return nil, errors.New("validation failed: " + strings.Join(validationErrs, ", "))
	}

	return cfg, nil
}
