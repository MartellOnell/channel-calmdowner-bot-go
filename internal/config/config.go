package config

import (
	"strconv"

	"github.com/MartellOnell/envscan"
)

type Config struct {
	BotToken      string `env:"BOT_TOKEN"`
	DBPath        string `env:"DB_PATH"`
	CheckInterval int    `env:"CHECK_INTERVAL"`
}

var defaults = map[string]string{
	"DB_PATH":        "bot_data.db",
	"CHECK_INTERVAL": "3600",
}

func Load() Config {
	var cfg Config
	if err := envscan.ReadEnvironment(&cfg, defaults); err != nil {
		panic("failed to load config: " + err.Error())
	}
	return cfg
}

func ExtractBotID(token string) int64 {
	for i, c := range token {
		if c == ':' {
			id, err := strconv.ParseInt(token[:i], 10, 64)
			if err == nil {
				return id
			}
			return 0
		}
	}
	return 0
}
