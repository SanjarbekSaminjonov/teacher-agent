package bot_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"teacher-agent/internal/ai"
	"teacher-agent/internal/bot"
	"teacher-agent/internal/config"
	"teacher-agent/internal/curriculum"
	"teacher-agent/internal/database"
)

func TestBuildDailyExecutiveReport(t *testing.T) {
	_ = godotenv.Load("../../.env")
	_ = godotenv.Load(".env")

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig err: %v", err)
	}
	cfg.DryRun = true

	dbPath := cfg.DatabasePath
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		dbPath = "../../bot.db"
	}

	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB err: %v", err)
	}
	defer db.Close()

	curr, err := curriculum.NewManager()
	if err != nil {
		t.Fatalf("NewManager err: %v", err)
	}

	aiCli := ai.NewClient(cfg.GeminiAPIKeys...)
	if !aiCli.IsConfigured() {
		t.Skip("Gemini API key not configured")
	}

	b, err := bot.NewBot(cfg, db, curr, aiCli)
	if err != nil {
		t.Fatalf("NewBot err: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	report, err := b.BuildDailyExecutiveReport(ctx)
	if err != nil {
		t.Fatalf("BuildDailyExecutiveReport err: %v", err)
	}

	if len(report) == 0 {
		t.Errorf("Hisobot bo'sh qaytdi")
	}

	t.Logf("Generatsiya qilingan hisobot:\n%s\n", report)
}
