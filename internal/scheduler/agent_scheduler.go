package scheduler

import (
	"fmt"
	"log"
	"time"

	"teacher-agent/internal/bot"
	"teacher-agent/internal/config"
	"teacher-agent/internal/curriculum"
	"teacher-agent/internal/database"

	"github.com/robfig/cron/v3"
)

type AgentScheduler struct {
	cron       *cron.Cron
	cfg        *config.Config
	db         *database.DB
	curriculum *curriculum.Manager
	bot        *bot.Bot
}

func NewAgentScheduler(cfg *config.Config, db *database.DB, curr *curriculum.Manager, b *bot.Bot) *AgentScheduler {
	// Use local timezone
	c := cron.New(cron.WithLocation(time.Local))

	s := &AgentScheduler{
		cron:       c,
		cfg:        cfg,
		db:         db,
		curriculum: curr,
		bot:        b,
	}

	s.setupCronJobs()
	return s
}

func (s *AgentScheduler) Start() {
	s.cron.Start()
	log.Println("⏰ Avtonom darslar jadvali (Agent Scheduler) faollashtirildi")

	// If group ID configured in .env, ensure it's registered
	if s.cfg.TelegramGroupID != 0 {
		_, _ = s.db.GetOrCreateGroupState(s.cfg.TelegramGroupID, "Go Study Group")
	}

	// Trigger immediate check in background to see if today's morning lesson is pending
	go s.CheckAndDispatchPending()
}

func (s *AgentScheduler) Stop() {
	s.cron.Stop()
}

func (s *AgentScheduler) setupCronJobs() {
	// Morning Lesson: e.g. 10:00 (Mon-Fri)
	morningSpec := fmt.Sprintf("0 %d * * 1-5", s.cfg.MorningHour)
	_, _ = s.cron.AddFunc(morningSpec, s.DispatchMorningLessons)

	// Afternoon Quiz: e.g. 13:00 (Mon-Fri)
	afternoonSpec := fmt.Sprintf("0 %d * * 1-5", s.cfg.AfternoonHour)
	_, _ = s.cron.AddFunc(afternoonSpec, s.DispatchAfternoonQuizzes)

	// Evening Challenge: e.g. 15:00 (Mon-Fri)
	eveningSpec := fmt.Sprintf("0 %d * * 1-5", s.cfg.EveningHour)
	_, _ = s.cron.AddFunc(eveningSpec, s.DispatchEveningChallenges)

	// Mid-afternoon Icebreaker for quiet/inactive members: 16:00 (Mon-Fri)
	_, _ = s.cron.AddFunc("0 16 * * 1-5", s.DispatchIcebreaker)

	// Evening Nudge: e.g. 17:00 (Mon-Fri)
	nudgeSpec := fmt.Sprintf("0 %d * * 1-5", s.cfg.NudgeHour)
	_, _ = s.cron.AddFunc(nudgeSpec, s.DispatchNudges)

	// Challenge Deadline: 17:30 (Mon-Fri)
	_, _ = s.cron.AddFunc("30 17 * * 1-5", s.DispatchChallengeDeadline)

	// Daily Executive Report to Admin: 18:00 (Mon-Fri)
	_, _ = s.cron.AddFunc("0 18 * * 1-5", s.DispatchDailyReport)

	// Weekend Wish: Saturday 10:00
	_, _ = s.cron.AddFunc("0 10 * * 6", s.DispatchWeekendWish)
}

func (s *AgentScheduler) CheckAndDispatchPending() {
	time.Sleep(3 * time.Second) // wait for bot startup
	today := time.Now().Format("2006-01-02")
	currentHour := time.Now().Hour()
	weekday := time.Now().Weekday()

	states, err := s.db.GetAllGroupStates()
	if err != nil {
		return
	}

	for _, state := range states {
		// Weekend handling:
		if weekday == time.Saturday || weekday == time.Sunday {
			if weekday == time.Saturday && !state.WeekendWishSent && currentHour >= 10 {
				_ = s.bot.SendWeekendWishToChat(state.ChatID)
				state.WeekendWishSent = true
				_ = s.db.UpdateGroupState(&state)
			}
			continue
		}

		// If today is marked as paused (holiday / rest day), skip!
		if state.PausedDate == today {
			continue
		}

		// Reset weekend wish on weekdays
		if state.WeekendWishSent {
			state.WeekendWishSent = false
			_ = s.db.UpdateGroupState(&state)
		}

		// If date changed, reset flags and optionally advance
		if state.LessonDate != today {
			if state.LessonDate != "" && state.MorningSent {
				// Advance to next lesson if previous day finished
				if state.CurrentLessonID < s.curriculum.TotalLessons() {
					state.CurrentLessonID++
				}
			}
			state.LessonDate = today
			state.MorningSent = false
			state.AfternoonQuizSent = false
			state.EveningChallengeSent = false
			state.NudgeSent = false
			state.DeadlineAnnounced = false
			state.IcebreakerSent = false
			_ = s.db.UpdateGroupState(&state)
		}

		lesson, exists := s.curriculum.GetLesson(state.CurrentLessonID)
		if !exists {
			continue
		}

		// Check what should be sent based on current time
		if currentHour >= s.cfg.MorningHour && !state.MorningSent {
			if err := s.bot.SendMorningLessonToChat(state.ChatID, lesson); err == nil {
				state.MorningSent = true
				_ = s.db.UpdateGroupState(&state)
			}
		}

		if currentHour >= s.cfg.AfternoonHour && state.MorningSent && !state.AfternoonQuizSent {
			if err := s.bot.SendQuizToChat(state.ChatID, lesson); err == nil {
				state.AfternoonQuizSent = true
				_ = s.db.UpdateGroupState(&state)
			}
		}

		if currentHour >= s.cfg.EveningHour && state.AfternoonQuizSent && !state.EveningChallengeSent {
			if err := s.bot.SendChallengeToChat(state.ChatID, lesson); err == nil {
				state.EveningChallengeSent = true
				_ = s.db.UpdateGroupState(&state)
			}
		}

		nowMin := time.Now().Minute()
		// Check pending Nudge (if >= 17:00 and < 17:30)
		if currentHour == s.cfg.NudgeHour && nowMin < 30 && state.EveningChallengeSent && !state.NudgeSent {
			if err := s.bot.SendNudgeToChat(state.ChatID, lesson); err == nil {
				state.NudgeSent = true
				_ = s.db.UpdateGroupState(&state)
				log.Printf("✅ Qolib ketgan 17:00 Nudge eslatmasi yuborildi: Chat %d\n", state.ChatID)
			}
		}

		// Check pending Deadline (if >= 17:30)
		if (currentHour > 17 || (currentHour == 17 && nowMin >= 30)) && state.EveningChallengeSent && !state.DeadlineAnnounced {
			if err := s.bot.SendChallengeDeadline(state.ChatID, lesson); err == nil {
				state.DeadlineAnnounced = true
				_ = s.db.UpdateGroupState(&state)
				log.Printf("🛑 Qolib ketgan 17:30 Deadline e'lon qilindi: Chat %d\n", state.ChatID)
			}
		}
	}
}

func (s *AgentScheduler) DispatchMorningLessons() {
	weekday := time.Now().Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return
	}

	today := time.Now().Format("2006-01-02")
	log.Printf("🌅 Ertalabki darslar yuborilmoqda: %s\n", today)

	states, err := s.db.GetAllGroupStates()
	if err != nil {
		log.Printf("Guruhlarni olishda xato: %v\n", err)
		return
	}

	for _, state := range states {
		if state.PausedDate == today {
			log.Printf("Guruh %d bugun to'xtatilgan (bayram/dam olish), o'tkazib yuboriladi.\n", state.ChatID)
			continue
		}

		if state.LessonDate != today {
			if state.LessonDate != "" && state.MorningSent {
				if state.CurrentLessonID < s.curriculum.TotalLessons() {
					state.CurrentLessonID++
				}
			}
			state.LessonDate = today
			state.MorningSent = false
			state.AfternoonQuizSent = false
			state.EveningChallengeSent = false
			state.NudgeSent = false
			state.DeadlineAnnounced = false
			state.IcebreakerSent = false
		}

		if state.MorningSent {
			continue
		}

		lesson, exists := s.curriculum.GetLesson(state.CurrentLessonID)
		if !exists {
			continue
		}

		if err := s.bot.SendMorningLessonToChat(state.ChatID, lesson); err != nil {
			log.Printf("Ertalabki dars yuborishda xato (chat %d): %v\n", state.ChatID, err)
		} else {
			state.MorningSent = true
			_ = s.db.UpdateGroupState(&state)
		}
	}
}

func (s *AgentScheduler) DispatchAfternoonQuizzes() {
	weekday := time.Now().Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return
	}

	today := time.Now().Format("2006-01-02")
	log.Printf("🥪 Tushlikdagi viktorinalar yuborilmoqda: %s\n", today)

	states, err := s.db.GetAllGroupStates()
	if err != nil {
		return
	}

	for _, state := range states {
		if state.PausedDate == today {
			continue
		}

		if !state.MorningSent || state.AfternoonQuizSent {
			continue
		}

		lesson, exists := s.curriculum.GetLesson(state.CurrentLessonID)
		if !exists {
			continue
		}

		if err := s.bot.SendQuizToChat(state.ChatID, lesson); err != nil {
			log.Printf("Quiz yuborishda xato (chat %d): %v\n", state.ChatID, err)
		} else {
			state.AfternoonQuizSent = true
			_ = s.db.UpdateGroupState(&state)
		}
	}
}

func (s *AgentScheduler) DispatchEveningChallenges() {
	weekday := time.Now().Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return
	}

	today := time.Now().Format("2006-01-02")
	log.Printf("🌙 Kechki kod topshiriqlari yuborilmoqda: %s\n", today)

	states, err := s.db.GetAllGroupStates()
	if err != nil {
		return
	}

	for _, state := range states {
		if state.PausedDate == today {
			continue
		}

		if !state.AfternoonQuizSent || state.EveningChallengeSent {
			continue
		}

		lesson, exists := s.curriculum.GetLesson(state.CurrentLessonID)
		if !exists {
			continue
		}

		if err := s.bot.SendChallengeToChat(state.ChatID, lesson); err != nil {
			log.Printf("Challenge yuborishda xato (chat %d): %v\n", state.ChatID, err)
		} else {
			state.EveningChallengeSent = true
			_ = s.db.UpdateGroupState(&state)
		}
	}
}

func (s *AgentScheduler) DispatchIcebreaker() {
	weekday := time.Now().Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return
	}

	log.Println("🎯 16:00 Passiv a'zolar uchun Icebreaker chaqiruvi tekshirilmoqda...")

	states, err := s.db.GetAllGroupStates()
	if err != nil {
		return
	}

	today := time.Now().Format("2006-01-02")

	for _, state := range states {
		if state.PausedDate == today || state.IcebreakerSent {
			continue
		}

		if !state.EveningChallengeSent {
			continue
		}

		lesson, exists := s.curriculum.GetLesson(state.CurrentLessonID)
		if !exists {
			continue
		}

		if err := s.bot.SendIcebreakerToChat(state.ChatID, lesson); err == nil {
			state.IcebreakerSent = true
			_ = s.db.UpdateGroupState(&state)
			log.Printf("🎯 Chat %d ga Icebreaker chaqiruvi yuborildi\n", state.ChatID)
		} else {
			log.Printf("Icebreaker yuborishda xato (chat %d): %v\n", state.ChatID, err)
		}
	}
}

func (s *AgentScheduler) DispatchNudges() {
	weekday := time.Now().Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return
	}

	log.Println("👀 17:00 Nudge eslatmasi yuborilmoqda...")

	states, err := s.db.GetAllGroupStates()
	if err != nil {
		return
	}

	today := time.Now().Format("2006-01-02")

	for _, state := range states {
		if state.PausedDate == today {
			continue
		}

		if !state.EveningChallengeSent || state.NudgeSent {
			continue
		}

		lesson, exists := s.curriculum.GetLesson(state.CurrentLessonID)
		if !exists {
			continue
		}

		if err := s.bot.SendNudgeToChat(state.ChatID, lesson); err == nil {
			state.NudgeSent = true
			_ = s.db.UpdateGroupState(&state)
			log.Printf("✅ 17:00 Nudge eslatmasi yuborildi: Chat %d\n", state.ChatID)
		}
	}
}

func (s *AgentScheduler) DispatchChallengeDeadline() {
	weekday := time.Now().Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return
	}

	log.Println("🛑 17:30 Challenge deadline e'lon qilinmoqda...")

	states, err := s.db.GetAllGroupStates()
	if err != nil {
		return
	}

	today := time.Now().Format("2006-01-02")

	for _, state := range states {
		if state.PausedDate == today {
			continue
		}

		if !state.EveningChallengeSent || state.DeadlineAnnounced {
			continue
		}

		lesson, exists := s.curriculum.GetLesson(state.CurrentLessonID)
		if !exists {
			continue
		}

		if err := s.bot.SendChallengeDeadline(state.ChatID, lesson); err == nil {
			state.DeadlineAnnounced = true
			_ = s.db.UpdateGroupState(&state)
			log.Printf("🛑 17:30 Challenge deadline e'lon qilindi: Chat %d\n", state.ChatID)
		}
	}
}

func (s *AgentScheduler) DispatchDailyReport() {
	weekday := time.Now().Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return
	}

	today := time.Now().Format("2006-01-02")
	states, err := s.db.GetAllGroupStates()
	if err == nil {
		allPaused := true
		for _, st := range states {
			if st.PausedDate != today {
				allPaused = false
				break
			}
		}
		if len(states) > 0 && allPaused {
			log.Println("Guruhlar bugungi kunga to'xtatilgan, hisobot o'tkazib yuboriladi.")
			return
		}
	}

	log.Println("📊 Kunlik tahliliy hisobot yuborish vaqti keldi (18:00)...")
	if err := s.bot.SendDailyReportToAdmin(); err != nil {
		log.Printf("Kunlik hisobotni adminga yuborishda xato: %v\n", err)
	}
}

func (s *AgentScheduler) DispatchWeekendWish() {
	weekday := time.Now().Weekday()
	if weekday != time.Saturday && weekday != time.Sunday {
		return
	}

	log.Println("🏖️ Dam olish kuni tilagi guruhlarga yuborilmoqda...")
	states, err := s.db.GetAllGroupStates()
	if err != nil {
		return
	}

	for _, state := range states {
		if !state.WeekendWishSent {
			if err := s.bot.SendWeekendWishToChat(state.ChatID); err == nil {
				state.WeekendWishSent = true
				_ = s.db.UpdateGroupState(&state)
			}
		}
	}
}


