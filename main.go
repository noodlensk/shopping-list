package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/noodlensk/shopping-list/internal/common/config"
	"github.com/noodlensk/shopping-list/internal/common/logs"
	"github.com/noodlensk/shopping-list/internal/grocery/ports"
	"github.com/noodlensk/shopping-list/internal/grocery/service"
	"go.uber.org/zap"
)

type stringList []string

func (i *stringList) String() string { return strings.Join(*i, ", ") }

func (i *stringList) Set(value string) error {
	*i = append(*i, value)

	return nil
}

var configFiles stringList

func main() {
	logger := logs.NewLogger()

	flag.Var(&configFiles, "config", "Path to config file")
	flag.Parse()

	if err := run(logger); err != nil {
		logger.Fatal(err)
	}
}

func run(logger *zap.SugaredLogger) error {
	cfg, err := config.Parse(configFiles...)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	db, err := bolt.Open(cfg.Storage.Path, os.FileMode(0o600), nil)
	if err != nil {
		return err
	}

	app, err := service.NewApplication(db)
	if err != nil {
		return err
	}

	return ports.NewTelegram(cfg.Telegram.Token, cfg.Telegram.AllowedUsers, app, logger)
}
