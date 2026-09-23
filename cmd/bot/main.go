package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"learn-go-bot/internal/ai"
	"learn-go-bot/internal/bot"
	"learn-go-bot/internal/config"
	"learn-go-bot/internal/curriculum"
	"learn-go-bot/internal/database"
	"learn-go-bot/internal/scheduler"
)

func main() {
	log.Println("🚀 Go Guruhi O'qituvchi Bot tizimi ishga tushmoqda...")

	// 1. Config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Konfiguratsiyani yuklashda xato: %v\n", err)
	}

	if cfg.TelegramBotToken == "" && !cfg.DryRun {
		log.Println("⚠️ OGOHLANTIRISH: TELEGRAM_BOT_TOKEN kiritilmagan!")
		log.Println("Botni tokensiz sinash uchun .env faylida DRY_RUN=true deb sozlang yoki tokenni kiriting.")
		os.Exit(1)
	}

	// 2. Database
	db, err := database.NewDB(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Ma'lumotlar bazasini initsializatsiya qilishda xato: %v\n", err)
	}
	defer db.Close()
	log.Printf("📦 Ma'lumotlar bazasi tayyor: %s\n", cfg.DatabasePath)

	// 3. Curriculum
	currManager, err := curriculum.NewManager()
	if err != nil {
		log.Fatalf("O'quv rejasini yuklashda xato: %v\n", err)
	}
	log.Printf("📚 Jami %d ta Go darsi muvaffaqiyatli yuklandi.\n", currManager.TotalLessons())

	// 4. Gemini AI Client
	aiClient := ai.NewClient(cfg.GeminiAPIKeys...)
	if aiClient.IsConfigured() {
		log.Printf("🤖 Google Gemini AI moduli faollashtirildi (Jami %d ta API kalit pool)\n", len(cfg.GeminiAPIKeys))
		aiClient.SetLogFunc(func(actionType, model, prompt, response string, durationMs int64, errMsg string) {
			_ = db.SaveAILog(actionType, model, prompt, response, durationMs, errMsg)
		})
	} else {
		log.Println("ℹ️ GEMINI_API_KEY kiritilmagan: Kod tekshirish standart rejimda ishlaydi")
	}

	// 5. Telegram Bot
	telegramBot, err := bot.NewBot(cfg, db, currManager, aiClient)
	if err != nil {
		log.Fatalf("Telegram botni ishga tushirishda xato: %v\n", err)
	}

	// 6. Agent Scheduler
	sched := scheduler.NewAgentScheduler(cfg, db, currManager, telegramBot)
	sched.Start()
	defer sched.Stop()

	// Graceful shutdown handling
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stopChan
		log.Println("🛑 Tizim to'xtatilmoqda...")
		telegramBot.Stop()
		sched.Stop()
		_ = db.Close()
		log.Println("✅ Bot xavfsiz to'xtatildi.")
		os.Exit(0)
	}()

	// Start bot poller
	telegramBot.Start()
}
