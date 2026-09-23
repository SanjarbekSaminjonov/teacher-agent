package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramBotToken string
	GeminiAPIKey     string
	GeminiAPIKeys    []string
	TelegramGroupID  int64
	AdminTelegramID  int64
	DatabasePath     string
	MorningHour      int
	AfternoonHour    int
	EveningHour      int
	NudgeHour        int
	DryRun           bool
}

func LoadConfig() (*Config, error) {
	// Try loading .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("Note: .env file not found or couldn't be loaded, using environment variables")
	}

	cfg := &Config{
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		GeminiAPIKey:     os.Getenv("GEMINI_API_KEY"),
		DatabasePath:     getEnv("DATABASE_PATH", "./bot.db"),
		MorningHour:      getEnvInt("MORNING_HOUR", 10),
		AfternoonHour:    getEnvInt("AFTERNOON_HOUR", 13),
		EveningHour:      getEnvInt("EVENING_HOUR", 15),
		NudgeHour:        getEnvInt("NUDGE_HOUR", 17),
		DryRun:           getEnvBool("DRY_RUN", false),
	}

	keysStr := os.Getenv("GEMINI_API_KEYS")
	var keys []string
	if keysStr != "" {
		for _, k := range strings.Split(keysStr, ",") {
			trimmed := strings.TrimSpace(k)
			if trimmed != "" {
				keys = append(keys, trimmed)
			}
		}
	}
	if len(keys) == 0 && cfg.GeminiAPIKey != "" {
		keys = append(keys, cfg.GeminiAPIKey)
	}
	cfg.GeminiAPIKeys = keys
	if len(keys) > 0 {
		cfg.GeminiAPIKey = keys[0]
	}

	if groupIDStr := os.Getenv("TELEGRAM_GROUP_ID"); groupIDStr != "" {
		if id, err := strconv.ParseInt(groupIDStr, 10, 64); err == nil {
			cfg.TelegramGroupID = id
		}
	}

	if adminIDStr := os.Getenv("ADMIN_TELEGRAM_ID"); adminIDStr != "" {
		if id, err := strconv.ParseInt(adminIDStr, 10, 64); err == nil {
			cfg.AdminTelegramID = id
		}
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}
