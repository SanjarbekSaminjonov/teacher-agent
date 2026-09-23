package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"teacher-agent/internal/ai"
	"teacher-agent/internal/config"
	"teacher-agent/internal/curriculum"
	"teacher-agent/internal/database"

	tele "gopkg.in/telebot.v3"
)

type Bot struct {
	teleBot            *tele.Bot
	cfg                *config.Config
	db                 *database.DB
	curriculum         *curriculum.Manager
	aiClient           *ai.Client
	debouncer          *ChatDebouncer
	quotaExceededToday bool
	quotaExceededDate  string
	quotaMu            sync.Mutex
}

func NewBot(cfg *config.Config, db *database.DB, curr *curriculum.Manager, aiCli *ai.Client) (*Bot, error) {
	if cfg.DryRun {
		log.Println("DRY_RUN rejimi: Telegram bot tarmoqqa ulanmasdan ishga tushirildi")
		return &Bot{
			cfg:        cfg,
			db:         db,
			curriculum: curr,
			aiClient:   aiCli,
		}, nil
	}

	pref := tele.Settings{
		Token:  cfg.TelegramBotToken,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("telebot yaratishda xato: %w", err)
	}

	appBot := &Bot{
		teleBot:    b,
		cfg:        cfg,
		db:         db,
		curriculum: curr,
		aiClient:   aiCli,
	}

	appBot.debouncer = NewChatDebouncer(appBot, 15*time.Minute)
	appBot.registerHandlers()
	return appBot, nil
}

func (b *Bot) Start() {
	if b.teleBot != nil {
		log.Printf("🤖 Go O'qituvchi Bot (@%s) ishga tushdi!\n", b.teleBot.Me.Username)
		b.teleBot.Start()
	} else {
		log.Println("🤖 Bot simulatsiya rejimida ishlayapti.")
	}
}

func (b *Bot) Stop() {
	if b.teleBot != nil {
		b.teleBot.Stop()
	}
}

func (b *Bot) GetTeleBot() *tele.Bot {
	return b.teleBot
}

func (b *Bot) registerHandlers() {
	b.teleBot.Handle("/start", b.handleStart)
	b.teleBot.Handle("/help", b.handleHelp)
	b.teleBot.Handle("/today", b.handleToday)
	b.teleBot.Handle("/quiz", b.handleQuizCommand)
	b.teleBot.Handle("/challenge", b.handleChallengeCommand)
	b.teleBot.Handle("/leaderboard", b.handleLeaderboard)
	b.teleBot.Handle("/progress", b.handleProgress)
	b.teleBot.Handle("/next", b.handleNextLesson)
	b.teleBot.Handle("/nudge", b.handleNudgeCommand)
	b.teleBot.Handle("/ask", b.handleAsk)

	// Admin & Report commands
	b.teleBot.Handle("/report", b.handleReportCommand)
	b.teleBot.Handle("/ai_logs", b.handleAILogsCommand)
	b.teleBot.Handle("/pause_today", b.handlePauseToday)
	b.teleBot.Handle("/resume_today", b.handleResumeToday)

	// Poll Answer handler for Quizzes
	b.teleBot.Handle(tele.OnPollAnswer, b.handlePollAnswer)

	// General text handler for code review and questions
	b.teleBot.Handle(tele.OnText, b.handleText)

	// Group join/leave handlers
	b.teleBot.Handle(tele.OnAddedToGroup, b.handleGroupJoined)
	b.teleBot.Handle(tele.OnUserJoined, b.handleUserJoined)
	b.teleBot.Handle(tele.OnUserLeft, b.handleUserLeft)
	b.teleBot.Handle(tele.OnMigration, b.handleMigration)
}

func (b *Bot) handleStart(c tele.Context) error {
	msg := `👋 **Assalomu alaykum, Go o'rganuvchilar!**

Men — Go (Golang) dasturlash tili bo'yicha **avtonom o'qituvchi-agent**man 🐹.

Mening vazifam:
1. Sizga har kuni Go tilining yangi mavzusini lo'nda va amaliy tushuntirib borish.
2. Tushlikda mavzu yuzasidan interaktiv Telegram Quiz (viktorina) o'tkazish.
3. Kechqurun mini-topshiriq (challenge) berish va siz yuborgan kodlarni Gemini AI orqali tekshirish.
4. Eng faol o'rganuvchilarga ball berib, umumiy reytingni (Leaderboard) yuritish!

📌 **Asosiy buyruqlar:**
• /today — Bugungi dars va topshiriq
• /quiz — Bugungi viktorina
• /challenge — Bugungi amaliy kod topshirig'i
• /ask <savol> — Go bo'yicha botga savol berish
• /leaderboard — Guruh a'zolarining reytingi (TOP-10)
• /progress — Kurs bo'yicha umumiy o'zlashtirish holati
• /help — Yordam va ko'rsatmalar`

	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleHelp(c tele.Context) error {
	msg := `📚 **Botdan foydalanish bo'yicha qo'llanma:**

1. **Darslar qachon bo'ladi?**
   Bot avtonom ravishda har kuni ertalab nazariy darsni, tushda viktorinani va kechqurun topshiriqni guruhga yuboradi.

2. **Topshiriqni qanday topshirish kerak?**
   Kechki topshiriq xabariga javob (Reply) qilib Go kodingizni yuboring, yoki guruhga ` + "```go ... ```" + ` formatida kod tashlang. Bot kodingizni tahlil qilib, o'z bahosini va ballini beradi.

3. **Ballar qanday hisoblanadi?**
   - Viktorinaga to'g'ri javob: **+10 ball**
   - Kod topshirig'i yechimi: **+15...+20 ball**`

	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleToday(c tele.Context) error {
	state, err := b.db.GetOrCreateGroupState(c.Chat().ID, c.Chat().Title)
	if err != nil {
		return c.Send("Holatni yuklashda xatolik yuz berdi.")
	}

	lesson, exists := b.curriculum.GetLesson(state.CurrentLessonID)
	if !exists {
		return c.Send("Mavzu topilmadi.")
	}

	msg := fmt.Sprintf("📖 **Bugungi Mavzu (%d/%d):** %s\n\n%s\n\n---\n*Viktorinani ko'rish uchun:* /quiz\n*Kechki topshiriq uchun:* /challenge",
		lesson.ID, b.curriculum.TotalLessons(), lesson.Title, lesson.Theory)

	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleQuizCommand(c tele.Context) error {
	state, err := b.db.GetOrCreateGroupState(c.Chat().ID, c.Chat().Title)
	if err != nil {
		return c.Send("Holatni yuklashda xatolik yuz berdi.")
	}

	lesson, exists := b.curriculum.GetLesson(state.CurrentLessonID)
	if !exists {
		return c.Send("Mavzu topilmadi.")
	}

	return b.SendQuizToChat(c.Chat().ID, lesson)
}

func (b *Bot) handleChallengeCommand(c tele.Context) error {
	state, err := b.db.GetOrCreateGroupState(c.Chat().ID, c.Chat().Title)
	if err != nil {
		return c.Send("Holatni yuklashda xatolik yuz berdi.")
	}

	lesson, exists := b.curriculum.GetLesson(state.CurrentLessonID)
	if !exists {
		return c.Send("Mavzu topilmadi.")
	}

	return b.SendChallengeToChat(c.Chat().ID, lesson)
}

func (b *Bot) handleLeaderboard(c tele.Context) error {
	users, err := b.db.GetTopUsers(10)
	if err != nil {
		return c.Send("Reytingni yuklashda xatolik yuz berdi.")
	}

	if len(users) == 0 {
		return c.Send("🏆 Hozircha hech kim ball to'plamagan. Birinchi bo'lib viktorinada qatnashing yoki kod topshiring!")
	}

	var sb strings.Builder
	sb.WriteString("🏆 **Guruhning Eng Faol Go Dasturchilari:**\n\n")

	medals := []string{"🥇", "🥈", "🥉"}
	for i, u := range users {
		name := u.FirstName
		if u.Username != "" {
			name = fmt.Sprintf("%s (@%s)", name, u.Username)
		}

		icon := fmt.Sprintf("%d.", i+1)
		if i < len(medals) {
			icon = medals[i]
		}

		sb.WriteString(fmt.Sprintf("%s **%s** — %d ball (🧩 %d quiz, 💻 %d kod)\n",
			icon, name, u.Score, u.QuizzesSolved, u.ChallengesSolved))
	}

	return safeSendMarkdown(c, sb.String())
}

func (b *Bot) handleProgress(c tele.Context) error {
	state, err := b.db.GetOrCreateGroupState(c.Chat().ID, c.Chat().Title)
	if err != nil {
		return c.Send("Holatni yuklashda xatolik yuz berdi.")
	}

	total := b.curriculum.TotalLessons()
	current := state.CurrentLessonID

	percent := (current * 100) / total
	if percent > 100 {
		percent = 100
	}

	progressBar := makeProgressBar(current, total, 10)

	msg := fmt.Sprintf(`📊 **Go Kursi Bo'yicha Guruh Progressi:**

Mavzu: **%d / %d** (%d%%)
[%s]

Hammasi reja bo'yicha ketmoqda! Yangi mavzular va viktorinalarni o'tkazib yubormang 🚀`,
		current, total, percent, progressBar)

	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleNextLesson(c tele.Context) error {
	state, err := b.db.GetOrCreateGroupState(c.Chat().ID, c.Chat().Title)
	if err != nil {
		return c.Send("Holatni yuklashda xatolik yuz berdi.")
	}

	total := b.curriculum.TotalLessons()
	if state.CurrentLessonID >= total {
		return c.Send("🎉 Tabriklaymiz! Barcha 10 ta dars to'liq yakunlangan.")
	}

	state.CurrentLessonID++
	state.MorningSent = false
	state.AfternoonQuizSent = false
	state.EveningChallengeSent = false
	state.NudgeSent = false
	state.LessonDate = time.Now().Format("2006-01-02")

	if err := b.db.UpdateGroupState(state); err != nil {
		return c.Send("Holatni yangilashda xatolik yuz berdi.")
	}

	nextLesson, _ := b.curriculum.GetLesson(state.CurrentLessonID)
	return b.SendMorningLessonToChat(c.Chat().ID, nextLesson)
}

func (b *Bot) handleNudgeCommand(c tele.Context) error {
	state, err := b.db.GetOrCreateGroupState(c.Chat().ID, c.Chat().Title)
	if err != nil {
		return c.Send("Holatni yuklashda xatolik yuz berdi.")
	}

	lesson, exists := b.curriculum.GetLesson(state.CurrentLessonID)
	if !exists {
		return c.Send("Dars topilmadi.")
	}

	return b.SendNudgeToChat(c.Chat().ID, lesson)
}

func (b *Bot) getFormattedChatHistory(chatID int64, limit int) string {
	history, err := b.db.GetRecentChatHistory(chatID, limit)
	if err != nil || len(history) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, m := range history {
		sender := m.SenderName
		if m.IsBot {
			sender = "Bot (Teacher Agent)"
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", sender, m.MessageText))
	}
	return sb.String()
}

func (b *Bot) handleAsk(c tele.Context) error {
	args := c.Args()
	question := strings.TrimSpace(strings.Join(args, " "))

	if question == "" && c.Message().ReplyTo != nil && c.Message().ReplyTo.Text != "" {
		question = c.Message().ReplyTo.Text
	}

	if question == "" {
		return c.Reply("💡 Savolingizni kiriting.\nMasalan: `/ask Go tilida pointer nima uchun kerak?`", &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}

	state, err := b.db.GetOrCreateGroupState(c.Chat().ID, c.Chat().Title)
	if err != nil {
		return c.Send("Holatni yuklashda xato yuz berdi.")
	}

	lesson, _ := b.curriculum.GetLesson(state.CurrentLessonID)
	lessonTitle := "Go Asoslari"
	if lesson != nil {
		lessonTitle = lesson.Title
	}

	prevLessonTitle := ""
	if state.CurrentLessonID > 1 {
		if prevL, exists := b.curriculum.GetLesson(state.CurrentLessonID - 1); exists {
			prevLessonTitle = fmt.Sprintf("#%d: %s", prevL.ID, prevL.Title)
		}
	}

	activeLessonTitle := lessonTitle
	if !state.MorningSent && prevLessonTitle != "" {
		activeLessonTitle = fmt.Sprintf("%s (Takrorlash vaqti — yangi dars 10:00 da e'lon qilinadi)", prevLessonTitle)
	}

	replyContext := ""
	if c.Message().ReplyTo != nil && c.Message().ReplyTo.Text != "" {
		replyContext = c.Message().ReplyTo.Text
	}

	chatHistory := b.getFormattedChatHistory(c.Chat().ID, 8)

	senderName := c.Sender().FirstName
	if senderName == "" {
		senderName = c.Sender().Username
	}
	_ = b.db.SaveChatMessage(c.Chat().ID, senderName, question, false)

	_ = c.Notify(tele.Typing)
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	studyCtxStr := ""
	if lesson != nil {
		if sc, err := b.db.GetStudyContext(c.Chat().ID, lesson.ID, lesson.Title, prevLessonTitle, lesson.Challenge.Title, lesson.Challenge.Task, state.MorningSent, state.AfternoonQuizSent, state.EveningChallengeSent, state.DeadlineAnnounced, c.Sender().ID, senderName); err == nil {
			studyCtxStr = database.FormatStudyContext(sc)
		}
	}

	res, err := b.aiClient.AnswerQuestion(ctx, question, activeLessonTitle, replyContext, chatHistory, studyCtxStr)
	if err != nil {
		if ai.IsQuotaExceeded(err) {
			return b.handleQuotaExceeded(c)
		}
		log.Printf("AI /ask xatosi: %v\n", err)
		b.NotifyAdminError(fmt.Sprintf("/ask savol: \"%s\"", question), err)
		return c.Reply("Savolingizga javob shakllantirishda xatolik yuz berdi. Administratorga xabar yuborildi.")
	}

	_ = b.db.SaveChatMessage(c.Chat().ID, "Teacher Agent", res.Answer, true)
	return safeReplyMarkdown(c, res.Answer)
}

func (b *Bot) handleQuotaExceeded(c tele.Context) error {
	today := time.Now().Format("2006-01-02")
	b.quotaMu.Lock()
	alreadyNotified := b.quotaExceededToday && b.quotaExceededDate == today
	b.quotaExceededToday = true
	b.quotaExceededDate = today
	b.quotaMu.Unlock()

	if !alreadyNotified {
		b.NotifyAdminError("Google AI Studio Limiti", fmt.Errorf("bugungi kunlik Gemini API token/so'rov kvotasi (HTTP 429) yakunlandi"))

		msg := `🐹 **Do'stlar, bugungi Go mashg'ulotimiz uchun ajratilgan kunlik AI limiti o'z nihoyasiga yetdi!**

Bugun birgalikda ancha Go mavzulari va amaliyotlarini ko'rib chiqdik. Ertaga ertalab soat 10:00 da navbatdagi mavzu va yangi limit bilan darslarni davom ettiramiz! 🚀

Ungacha bugungi mavzuni takrorlab, guruhda o'zaro savol-javob qilishingiz va bir-biringizga yordam berishingiz mumkin. Hammaga omad! 💪`
		return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}

	return c.Reply("Bugungi kunlik AI limiti o'z nihoyasiga yetgan 🐹. Ertaga ertalab soat 10:00 da yangi dars va savol-javoblar bilan davom etamiz!")
}

func (b *Bot) handlePollAnswer(c tele.Context) error {
	ans := c.PollAnswer()
	if ans == nil || ans.Sender == nil || ans.Sender.ID == 0 {
		return nil
	}

	pollID := ans.PollID
	userID := ans.Sender.ID
	username := ans.Sender.Username
	firstName := ans.Sender.FirstName

	user, err := b.db.GetOrCreateUser(userID, username, firstName)
	if err != nil {
		log.Printf("Userni yaratishda xato: %v\n", err)
		return nil
	}

	states, err := b.db.GetAllGroupStates()
	if err != nil {
		return nil
	}

	var matchedState *database.GroupState
	for _, s := range states {
		if s.LastQuizPollID == pollID {
			matchedState = &s
			break
		}
	}

	if matchedState == nil {
		return nil
	}

	lesson, exists := b.curriculum.GetLesson(matchedState.CurrentLessonID)
	if !exists {
		return nil
	}

	// Check if user chose correct option
	isCorrect := false
	for _, optIdx := range ans.Options {
		if optIdx == lesson.Quiz.CorrectOptionID {
			isCorrect = true
			break
		}
	}

	firstAttempt, err := b.db.RecordQuizAttempt(userID, lesson.ID, isCorrect)
	if err != nil {
		log.Printf("Quiz attempt saqlashda xato: %v\n", err)
		return nil
	}

	if firstAttempt && isCorrect {
		_ = b.db.AddScore(userID, 10, true, false)
		log.Printf("Foydalanuvchi %s (%d) quizdan 10 ball oldi!\n", user.FirstName, userID)
	}

	return nil
}

func (b *Bot) handleText(c tele.Context) error {
	text := c.Text()
	sender := c.Sender()
	if sender == nil || sender.IsBot {
		return nil
	}

	// Save user in DB
	_, _ = b.db.GetOrCreateUser(sender.ID, sender.Username, sender.FirstName)

	// Save incoming message in chat history
	senderName := sender.FirstName
	if senderName == "" {
		senderName = sender.Username
	}
	_ = b.db.SaveChatMessage(c.Chat().ID, senderName, text, false)

	state, err := b.db.GetOrCreateGroupState(c.Chat().ID, c.Chat().Title)
	if err != nil {
		return nil
	}

	lesson, exists := b.curriculum.GetLesson(state.CurrentLessonID)
	if !exists {
		return nil
	}

	// Check if message is a reply to ANY message sent by the bot
	isReplyToBot := false
	replyTo := c.Message().ReplyTo
	if replyTo != nil && replyTo.Sender != nil && b.teleBot != nil && b.teleBot.Me != nil {
		if replyTo.Sender.ID == b.teleBot.Me.ID {
			isReplyToBot = true
		}
	}

	isReplyToChallenge := false
	if replyTo != nil && replyTo.ID == state.LastChallengeMessageID {
		isReplyToChallenge = true
	}

	lowerText := strings.ToLower(text)
	botMentioned := false
	if b.teleBot != nil && b.teleBot.Me != nil && b.teleBot.Me.Username != "" {
		if strings.Contains(lowerText, "@"+strings.ToLower(b.teleBot.Me.Username)) {
			botMentioned = true
		}
	}
	if strings.Contains(lowerText, "teacher agent") || strings.Contains(lowerText, "teacheragent") {
		botMentioned = true
	}

	today := time.Now().Format("2006-01-02")
	isAdmin := b.cfg.AdminTelegramID != 0 && sender.ID == b.cfg.AdminTelegramID
	if isAdmin {
		if strings.Contains(lowerText, "ertaga davom etamiz") ||
			strings.Contains(lowerText, "bugun dam olamiz") ||
			strings.Contains(lowerText, "bugun bayram") ||
			strings.Contains(lowerText, "bugun dars yo'q") ||
			strings.Contains(lowerText, "bugun dars bo'lmaydi") {
			state.PausedDate = today
			_ = b.db.UpdateGroupState(state)
			return safeReplyMarkdown(c, "Tushunarli! Bugungi dars va topshiriqlar to'xtatildi. Ertaga yangi kuch bilan darslarni davom ettiramiz! 🏖️🫡")
		}
	}

	isCode := looksLikeGoCode(text)

	// If it's a code submission (to challenge or code sent to bot)
	if isReplyToChallenge || (isReplyToBot && isCode) || isCode {
		return b.processCodeSubmission(c, lesson, text)
	}

	// If user is directly talking to the bot (replied to bot message or mentioned bot)
	if isReplyToBot || botMentioned {
		cleanQuestion := text
		if b.teleBot != nil && b.teleBot.Me != nil && b.teleBot.Me.Username != "" {
			cleanQuestion = strings.ReplaceAll(cleanQuestion, "@"+b.teleBot.Me.Username, "")
		}
		cleanQuestion = strings.TrimSpace(cleanQuestion)
		if cleanQuestion == "" {
			return c.Reply("Assalomu alaykum! Go dasturlash tili bo'yicha qanday savolingiz bor? Bemalol yozing! 🐹")
		}

		cleanLower := strings.ToLower(cleanQuestion)
		if strings.Contains(cleanLower, "kimda necha ball") ||
			strings.Contains(cleanLower, "kimda qancha ball") ||
			strings.Contains(cleanLower, "reyting") ||
			cleanLower == "ballar" ||
			strings.Contains(cleanLower, "leaderboard") ||
			strings.Contains(cleanLower, "peshqadam") {
			return b.handleLeaderboard(c)
		}
		if strings.Contains(cleanLower, "menda necha ball") ||
			strings.Contains(cleanLower, "menda qancha ball") ||
			strings.Contains(cleanLower, "mening ballim") ||
			strings.Contains(cleanLower, "ballim nechta") ||
			strings.Contains(cleanLower, "ballim qancha") {
			u, err := b.db.GetUser(sender.ID)
			if err == nil && u != nil {
				return safeReplyMarkdown(c, fmt.Sprintf("Salom, %s! Sizning jami to'plagan balingiz: **%d ball** (🧩 %d quiz, 💻 %d kod) 🚀", sender.FirstName, u.Score, u.QuizzesSolved, u.ChallengesSolved))
			}
		}

		replyContext := ""
		if replyTo != nil && replyTo.Text != "" {
			replyContext = replyTo.Text
		}

		chatHistory := b.getFormattedChatHistory(c.Chat().ID, 8)

		_ = c.Notify(tele.Typing)
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()

		prevLessonTitle := ""
		if state.CurrentLessonID > 1 {
			if prevL, exists := b.curriculum.GetLesson(state.CurrentLessonID - 1); exists {
				prevLessonTitle = fmt.Sprintf("#%d: %s", prevL.ID, prevL.Title)
			}
		}

		activeLessonTitle := lesson.Title
		if !state.MorningSent && prevLessonTitle != "" {
			activeLessonTitle = fmt.Sprintf("%s (Takrorlash vaqti — yangi dars 10:00 da e'lon qilinadi)", prevLessonTitle)
		}

		studyCtxStr := ""
		if sc, err := b.db.GetStudyContext(c.Chat().ID, lesson.ID, lesson.Title, prevLessonTitle, lesson.Challenge.Title, lesson.Challenge.Task, state.MorningSent, state.AfternoonQuizSent, state.EveningChallengeSent, state.DeadlineAnnounced, sender.ID, senderName); err == nil {
			studyCtxStr = database.FormatStudyContext(sc)
		}

		res, err := b.aiClient.AnswerQuestion(ctx, cleanQuestion, activeLessonTitle, replyContext, chatHistory, studyCtxStr)
		if err != nil {
			if ai.IsQuotaExceeded(err) {
				return b.handleQuotaExceeded(c)
			}
			log.Printf("AI savol-javobida xato: %v\n", err)
			b.NotifyAdminError(fmt.Sprintf("Guruh savoli: \"%s\"", cleanQuestion), err)
			return c.Reply("Savolingiz bo'yicha javob shakllantirishda xatolik yuz berdi. Administratorga xabar yuborildi.")
		}

		_ = b.db.SaveChatMessage(c.Chat().ID, "Teacher Agent", res.Answer, true)
		return safeReplyMarkdown(c, res.Answer)
	}

	// Buffer this chatter to summarize when quiet (if >= 2 events happen, AI will review and chime in)
	// Don't buffer if today is paused or weekend
	weekday := time.Now().Weekday()
	if weekday != time.Saturday && weekday != time.Sunday && state.PausedDate != today {
		if b.debouncer != nil {
			b.debouncer.AddEvent(c.Chat().ID, fmt.Sprintf("%s: \"%s\"", senderName, text), "")
			return nil
		}
	}

	return nil
}

func (b *Bot) processCodeSubmission(c tele.Context, lesson *curriculum.Lesson, code string) error {
	sender := c.Sender()
	_ = c.Notify(tele.Typing)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	senderName := sender.FirstName
	if senderName == "" {
		senderName = sender.Username
	}
	review, err := b.aiClient.ReviewCode(ctx, lesson.Title, lesson.Challenge.Task, code, senderName)
	now := time.Now()
	isAfterDeadline := now.Hour() > 17 || (now.Hour() == 17 && now.Minute() >= 30)

	if err != nil {
		if ai.IsQuotaExceeded(err) {
			if isAfterDeadline {
				_ = b.db.RecordSubmission(sender.ID, lesson.ID, code, "Topshiriq muddati (17:30) yakunlangan", 0)
				_ = c.Reply("Kodingiz qabul qilindi! ⚠️ Eslatma: Topshiriq muddati (17:30) yakunlanganligi sababli ball hisoblanmadi. 🚀")
				return b.handleQuotaExceeded(c)
			}
			oldMax, _ := b.db.GetMaxScoreForLesson(sender.ID, lesson.ID)
			diff := 15 - oldMax
			if diff > 0 {
				_ = b.db.AddScore(sender.ID, diff, false, oldMax == 0)
			}
			_ = b.db.RecordSubmission(sender.ID, lesson.ID, code, "AI kvotasi tugaganligi sababli ball avtomatik taqdim etildi", 15)
			_ = c.Reply("Kodingiz qabul qilindi va hisobingizga ball qo'shildi! 🚀")
			return b.handleQuotaExceeded(c)
		}
		log.Printf("Kod tahlilida xato: %v\n", err)
		b.NotifyAdminError(fmt.Sprintf("Kod tahlili xatosi (%s)", lesson.Title), err)
		return c.Reply("Kodingiz qabul qilindi, ammo AI tahlilida vaqtincha nosozlik yuz berdi. Ball hisoblandi! 🐹")
	}

	if isAfterDeadline {
		_ = b.db.RecordSubmission(sender.ID, lesson.ID, code, review.Feedback, 0)
		if review.DetectedBlunder != "" {
			_ = b.db.SaveUserNote(sender.ID, "blunder", review.DetectedBlunder)
		}
		replyMsg := fmt.Sprintf("🌟 **Kod Tahlili va Natija:**\n\n%s\n\n"+
			"⚠️ **Eslatma:** Bugungi topshiriqni qabul qilish muddati (soat 17:30) yakunlanganligi sababli ushbu yechim uchun reyting balli berilmaydi.\n"+
			"Lekin bilimingizni mustahkamlash uchun kod yozganingiz ajoyib! Mashq qilishda davom eting! 🚀", review.Feedback)

		_ = b.db.SaveChatMessage(c.Chat().ID, "Teacher Agent", replyMsg, true)
		return safeReplyMarkdown(c, replyMsg)
	}

	oldMaxScore, _ := b.db.GetMaxScoreForLesson(sender.ID, lesson.ID)
	diff := review.PointsEarned - oldMaxScore

	if diff > 0 {
		_ = b.db.AddScore(sender.ID, diff, false, oldMaxScore == 0)
	}
	_ = b.db.RecordSubmission(sender.ID, lesson.ID, code, review.Feedback, review.PointsEarned)

	if review.DetectedBlunder != "" {
		_ = b.db.SaveUserNote(sender.ID, "blunder", review.DetectedBlunder)
		log.Printf("📝 Mentor daftarchasiga kod xatosi qayd qilindi: User %d -> %s\n", sender.ID, review.DetectedBlunder)
	}

	var replyMsg string
	if oldMaxScore > 0 {
		if diff > 0 {
			replyMsg = fmt.Sprintf("🌟 **Kod Tahlili va Natija (Qayta topshirildi):**\n\n%s\n\n🎯 **Natija yangilandi:** Oldingi: %d ball ➡️ Yangi: %d ball (+%d ball qo'shildi)!\nBarakalla, xatolar ustida ishlab natijani oshirdingiz! 🚀",
				review.Feedback, oldMaxScore, review.PointsEarned, diff)
		} else {
			replyMsg = fmt.Sprintf("🌟 **Kod Tahlili va Natija (Qayta topshirildi):**\n\n%s\n\n🎯 **Ushbu urinish uchun baho:** %d ball.\nSizning oldingi eng yaxshi natijangiz (%d ball) saqlanib qoldi. 🚀",
				review.Feedback, review.PointsEarned, oldMaxScore)
		}
	} else {
		replyMsg = fmt.Sprintf("🌟 **Kod Tahlili va Natija:**\n\n%s\n\n🎯 **To'plangan ball:** +%d ball!\nBarakalla, o'rganishda davom eting! 🚀",
			review.Feedback, review.PointsEarned)
	}

	_ = b.db.SaveChatMessage(c.Chat().ID, "Teacher Agent", replyMsg, true)
	return safeReplyMarkdown(c, replyMsg)
}

func (b *Bot) handleGroupJoined(c tele.Context) error {
	state, err := b.db.GetOrCreateGroupState(c.Chat().ID, c.Chat().Title)
	if err != nil {
		log.Printf("Guruhni saqlashda xato: %v\n", err)
	} else {
		log.Printf("Bot yangi guruhga qo'shildi: %s (ID: %d)\n", state.GroupTitle, state.ChatID)
	}

	welcome := `👋 **Assalomu alaykum Go dasturchilari!**

Men ushbu guruhga Go dasturlash tilini birgalikda, bosqichma-bosqich va interaktiv tarzda o'rganish uchun qo'shildim 🐹.

Har kuni quyidagi tartibda ishlaymiz:
🌅 **Ertalab:** Yangi Go mavzusi va tushuntirish
🥪 **Tushda:** Bilimlarni sinash uchun viktorina (Quiz)
🌙 **Kechqurun:** Mini kod topshirig'i va AI tahlili

Birinchi darsni boshlash uchun /today buyrug'ini bosing!`

	return c.Send(welcome, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleUserJoined(c tele.Context) error {
	joinedUser := c.Message().UserJoined
	if joinedUser == nil || joinedUser.IsBot {
		return nil
	}

	// Save or update user in database immediately
	_, err := b.db.GetOrCreateUser(joinedUser.ID, joinedUser.Username, joinedUser.FirstName)
	if err != nil {
		log.Printf("Yangi a'zoni bazaga saqlashda xato: %v\n", err)
	} else {
		log.Printf("👤 Yangi guruh a'zosi ro'yxatga olindi: %s (@%s, ID: %d)\n", joinedUser.FirstName, joinedUser.Username, joinedUser.ID)
	}

	name := joinedUser.FirstName
	if joinedUser.Username != "" {
		name = fmt.Sprintf("%s (@%s)", joinedUser.FirstName, joinedUser.Username)
	}

	welcomeMsg := fmt.Sprintf(`👋 **Xush kelibsiz, %s!**

Bizning Go (Golang) o'quv guruhimizga xush kelibsiz 🐹.

Guruhimizda har kuni:
🌅 **10:00** — Kunlik Go darsi va nazariya
🥪 **13:00** — Bilimlarni sinash uchun viktorina (Quiz)
💻 **15:00** — Mini amaliy kod topshirig'i (AI tahlili bilan)
👀 **17:00** — Eslatma va savol-javoblar

Guruhda bemalol faol bo'ling, savollaringiz bo'lsa tortinmasdan so'rang! 🚀`, name)

	if b.debouncer != nil {
		b.debouncer.AddEvent(c.Chat().ID, fmt.Sprintf("%s guruhga yangi a'zo sifatida qo'shildi", name), welcomeMsg)
		return nil
	}

	return safeSendMarkdown(c, welcomeMsg)
}

func (b *Bot) handleUserLeft(c tele.Context) error {
	leftUser := c.Message().UserLeft
	if leftUser == nil || leftUser.IsBot {
		return nil
	}

	name := leftUser.FirstName
	if leftUser.Username != "" {
		name = fmt.Sprintf("%s (@%s)", leftUser.FirstName, leftUser.Username)
	}
	log.Printf("👋 A'zo guruhdan chiqdi: %s\n", name)
	if err := b.db.MarkUserLeft(leftUser.ID); err != nil {
		log.Printf("⚠️ A'zoni 'is_left' deb belgilashda xatolik: %v\n", err)
	}

	if b.debouncer != nil {
		b.debouncer.AddEvent(c.Chat().ID, fmt.Sprintf("%s guruhdan chiqib ketdi", name), "")
	}
	return nil
}

func (b *Bot) handleMigration(c tele.Context) error {
	from := c.Message().MigrateFrom
	to := c.Message().MigrateTo
	if to == 0 {
		to = c.Chat().ID
	}
	log.Printf("🔄 Telegram guruh migratsiyasi aniqlandi: %d -> %d\n", from, to)
	if err := b.db.MigrateGroupState(from, to); err != nil {
		log.Printf("⚠️ Guruh holatini migratsiya qilishda xato: %v\n", err)
		return err
	}
	log.Printf("✅ Guruh holati yangi superguruhga muvaffaqiyatli ko'chirildi: %d\n", to)
	return nil
}

func (b *Bot) SendMorningLessonToChat(chatID int64, lesson *curriculum.Lesson) error {
	if b.teleBot == nil {
		log.Printf("[SIMULATION] Ertalabki dars yuborildi chat: %d, dars: %s\n", chatID, lesson.Title)
		return nil
	}

	msg := fmt.Sprintf("🌅 **Kunlik Go Darsi (%d/%d):** %s\n\n%s\n\n💬 *Savollaringiz bo'lsa, bemalol yozing yoki botni tag qiling! Tushlikda mavzu bo'yicha viktorina bo'ladi.* 🐹",
		lesson.ID, b.curriculum.TotalLessons(), lesson.Title, lesson.Theory)

	target := &tele.Chat{ID: chatID}
	sentMsg, err := safeSendMarkdownDirect(b.teleBot, target, msg)
	if err != nil {
		return err
	}
	if sentMsg != nil {
		_ = b.teleBot.Pin(sentMsg)
	}
	return nil
}

func (b *Bot) SendQuizToChat(chatID int64, lesson *curriculum.Lesson) error {
	if b.teleBot == nil {
		log.Printf("[SIMULATION] Quiz yuborildi chat: %d, savol: %s\n", chatID, lesson.Quiz.Question)
		return nil
	}

	var pollOpts []tele.PollOption
	for _, opt := range lesson.Quiz.Options {
		pollOpts = append(pollOpts, tele.PollOption{Text: opt})
	}

	quiz := &tele.Poll{
		Type:          tele.PollQuiz,
		Question:      fmt.Sprintf("🥪 [%d-Mavzu] %s", lesson.ID, lesson.Quiz.Question),
		Options:       pollOpts,
		CorrectOption: lesson.Quiz.CorrectOptionID,
		Explanation:   lesson.Quiz.Explanation,
		Anonymous:     false, // Allows tracking who answered to give points!
	}

	target := &tele.Chat{ID: chatID}
	sentMsg, err := b.teleBot.Send(target, quiz)
	if err != nil {
		return err
	}

	_ = b.teleBot.Pin(sentMsg)

	if sentMsg.Poll != nil {
		state, err := b.db.GetOrCreateGroupState(chatID, "")
		if err == nil {
			state.LastQuizPollID = sentMsg.Poll.ID
			_ = b.db.UpdateGroupState(state)
		}
	}

	return nil
}

func (b *Bot) SendChallengeToChat(chatID int64, lesson *curriculum.Lesson) error {
	if b.teleBot == nil {
		log.Printf("[SIMULATION] Challenge yuborildi chat: %d, topshiriq: %s\n", chatID, lesson.Challenge.Title)
		return nil
	}

	var hintsText string
	if len(lesson.Challenge.Hints) > 0 {
		hintsText = "\n\n💡 **Maslahat (Hints):**\n"
		for _, h := range lesson.Challenge.Hints {
			hintsText += fmt.Sprintf("• %s\n", h)
		}
	}

	msg := fmt.Sprintf("🌙 **Kechki Amaliy Topshiriq (%d-Dars):** %s\n\n%s%s\n\n👉 *Yozgan kodingizni ushbu xabarga javob (reply) qilib yuboring! AI kodingizni tekshirib ball beradi.* 🐹",
		lesson.ID, lesson.Challenge.Title, lesson.Challenge.Task, hintsText)

	target := &tele.Chat{ID: chatID}
	formatted := MarkdownToTelegramHTML(msg)
	sentMsg, err := b.teleBot.Send(target, formatted, &tele.SendOptions{ParseMode: tele.ModeHTML})
	if err != nil {
		sentMsg, err = b.teleBot.Send(target, msg)
	}
	if err != nil {
		return err
	}

	_ = b.teleBot.Pin(sentMsg)

	state, err := b.db.GetOrCreateGroupState(chatID, "")
	if err == nil {
		state.LastChallengeMessageID = sentMsg.ID
		_ = b.db.UpdateGroupState(state)
	}

	return nil
}

func (b *Bot) SendNudgeToChat(chatID int64, lesson *curriculum.Lesson) error {
	if b.teleBot == nil {
		log.Printf("[SIMULATION] Nudge yuborildi chat: %d\n", chatID)
		return nil
	}

	target := &tele.Chat{ID: chatID}

	msg := fmt.Sprintf("⏳ **Kechki Topshiriq Eslatmasi (Deadline — 17:30)!**\n\n"+
		"Bugungi **#%d: \"%s\"** darsi topshirig'ini topshirish uchun oz vaqt qoldi!\n\n"+
		"Hali topshirmagan o'quvchilar, imkoniyatni boy bermay kodingizni guruhga yuboring va ballaringizni qo'lga kiriting! 💻🚀\n\n"+
		"⚠️ *Eslatma: Soat 17:30 dan keyin yuborilgan yechimlar tekshiriladi, ammo reyting balli berilmaydi.*",
		lesson.ID, lesson.Title)

	// Bugun hali topshirmagan 2 nafar o'quvchini do'stona chaqirish
	unsubmitted, err := b.db.GetUnsubmittedUsersForToday(lesson.ID, b.cfg.AdminTelegramID, 2)
	if err == nil && len(unsubmitted) > 0 {
		var mentions []string
		for _, u := range unsubmitted {
			if u.Username != "" {
				mentions = append(mentions, "@"+u.Username)
			} else if u.FirstName != "" {
				mentions = append(mentions, u.FirstName)
			}
		}
		if len(mentions) > 0 {
			msg += fmt.Sprintf("\n\n🎯 %s — sizlardan ham yechim kutib qolamiz, ozgina vaqt qoldi, qani boshladikmi? 😉", strings.Join(mentions, ", "))
		}
	}

	_ = b.db.SaveChatMessage(chatID, "Teacher Agent", msg, true)
	_, err = safeSendMarkdownDirect(b.teleBot, target, msg)
	return err
}

func (b *Bot) SendIcebreakerToChat(chatID int64, lesson *curriculum.Lesson) error {
	if b.teleBot == nil {
		log.Printf("[SIMULATION] Icebreaker yuborildi chat: %d\n", chatID)
		return nil
	}

	inactiveUsers, err := b.db.GetInactiveUsers(b.cfg.AdminTelegramID, 5)
	if err != nil || len(inactiveUsers) == 0 {
		return nil
	}

	var candidate *database.User
	for i := range inactiveUsers {
		if inactiveUsers[i].Username != "" {
			candidate = &inactiveUsers[i]
			break
		}
	}
	if candidate == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	icebreakerText, err := b.aiClient.GenerateIcebreaker(ctx, candidate.FirstName, candidate.Username, lesson.Title, "")
	if err != nil || strings.TrimSpace(icebreakerText) == "" {
		return err
	}

	target := &tele.Chat{ID: chatID}
	_ = b.db.SaveChatMessage(chatID, "Teacher Agent", icebreakerText, true)
	_, err = safeSendMarkdownDirect(b.teleBot, target, icebreakerText)
	return err
}

func (b *Bot) SendChallengeDeadline(chatID int64, lesson *curriculum.Lesson) error {
	if b.teleBot == nil {
		log.Printf("[SIMULATION] Challenge deadline yuborildi chat: %d\n", chatID)
		return nil
	}

	target := &tele.Chat{ID: chatID}

	topUsers, err := b.db.GetTopUsers(5)
	var lbLines []string
	if err == nil && len(topUsers) > 0 {
		medals := []string{"🥇", "🥈", "🥉"}
		for i, u := range topUsers {
			name := u.FirstName
			if u.Username != "" {
				name = fmt.Sprintf("%s (@%s)", name, u.Username)
			}
			icon := fmt.Sprintf("%d.", i+1)
			if i < len(medals) {
				icon = medals[i]
			}
			lbLines = append(lbLines, fmt.Sprintf("%s **%s** — %d ball (💻 %d kod)", icon, name, u.Score, u.ChallengesSolved))
		}
	}

	lbText := strings.Join(lbLines, "\n")
	if lbText == "" {
		lbText = "Hozircha ball to'plaganlar yo'q."
	}

	msg := fmt.Sprintf("🛑 **Bugungi Topshiriq Qabul Qilish Yakunlandi (17:30)!**\n\n"+
		"Bugungi **#%d: \"%s\"** darsi bo'yicha amaliy kod topshiriqlari qabul qilish muddati o'z nihoyasiga yetdi.\n\n"+
		"Faol qatnashib, o'z bilimlarini amalda ko'rsatgan barcha o'quvchilarimizga tasanno! 👏\n\n"+
		"🏆 **Bugungi Yakuniy Reyting:**\n%s\n\n"+
		"Soat 18:00 da kun yakunlari bo'yicha to'liq tahliliy hisobot tayyorlanadi. Ertaga soat 10:00 da yangi mavzuda ko'rishguncha! 🐹🚀",
		lesson.ID, lesson.Title, lbText)

	_ = b.db.SaveChatMessage(chatID, "Teacher Agent", msg, true)
	sentMsg, err := safeSendMarkdownDirect(b.teleBot, target, msg)
	if err != nil {
		return err
	}
	if sentMsg != nil {
		_ = b.teleBot.Pin(sentMsg)
	}
	return nil
}

func looksLikeGoCode(text string) bool {
	keywords := []string{
		"package main",
		"func main()",
		"fmt.Print",
		":=",
		"import (",
		"type ",
		"struct {",
		"interface {",
		"go func()",
		"chan ",
		"```go",
	}

	matchCount := 0
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			matchCount++
		}
	}

	return matchCount >= 2 || strings.Contains(text, "```go") || strings.Contains(text, "package main")
}

func makeProgressBar(current, total, length int) string {
	filled := (current * length) / total
	if filled > length {
		filled = length
	}
	empty := length - filled
	return strings.Repeat("🟩", filled) + strings.Repeat("⬜", empty)
}

func (b *Bot) NotifyAdminError(contextStr string, err error) {
	if b.teleBot == nil || b.cfg.AdminTelegramID == 0 || err == nil {
		return
	}

	msg := fmt.Sprintf("⚠️ **Botda Xatolik Yuz Berdi!**\n\n🕒 **Vaqt:** %s\n📍 **Kontekst:** %s\n❌ **Xato:**\n`%s`",
		time.Now().Format("2006-01-02 15:04:05"),
		contextStr,
		err.Error(),
	)

	target := &tele.User{ID: b.cfg.AdminTelegramID}
	formatted := MarkdownToTelegramHTML(msg)
	_, sendErr := b.teleBot.Send(target, formatted, &tele.SendOptions{ParseMode: tele.ModeHTML})
	if sendErr != nil {
		_, sendErr = b.teleBot.Send(target, msg)
		if sendErr != nil {
			log.Printf("Adminga xatolikni yuborishda xato: %v\n", sendErr)
		}
	}
}

func safeReplyMarkdown(c tele.Context, text string) error {
	formatted := MarkdownToTelegramHTML(text)
	err := c.Reply(formatted, &tele.SendOptions{ParseMode: tele.ModeHTML})
	if err != nil {
		log.Printf("HTML reply xatosi, oddiy matn yuborilmoqda: %v\n", err)
		return c.Reply(text)
	}
	return nil
}

func safeSendMarkdown(c tele.Context, text string) error {
	formatted := MarkdownToTelegramHTML(text)
	err := c.Send(formatted, &tele.SendOptions{ParseMode: tele.ModeHTML})
	if err != nil {
		log.Printf("HTML send xatosi, oddiy matn yuborilmoqda: %v\n", err)
		return c.Send(text)
	}
	return nil
}

func (b *Bot) handleReportCommand(c tele.Context) error {
	if b.cfg.AdminTelegramID != 0 && c.Sender().ID != b.cfg.AdminTelegramID {
		return c.Reply("⛔ Bu buyruq faqat bot administratori uchun mo'ljallangan.")
	}

	_ = c.Reply("⏳ Bugungi kunlik tahliliy hisobot tayyorlanmoqda, iltimos kuting...")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	report, err := b.BuildDailyExecutiveReport(ctx)
	if err != nil {
		return c.Reply(fmt.Sprintf("❌ Hisobot tuzishda xatolik yuz berdi: %v", err))
	}

	header := "📊 **Kunlik Tahliliy Hisobot (Admin uchun)**\n\n"
	return safeReplyMarkdown(c, header+report)
}

func (b *Bot) handleAILogsCommand(c tele.Context) error {
	if b.cfg.AdminTelegramID != 0 && c.Sender().ID != b.cfg.AdminTelegramID {
		return c.Reply("⛔ Bu buyruq faqat bot administratori uchun mo'ljallangan.")
	}

	todayCount, err := b.db.GetTodayAICount()
	if err != nil {
		todayCount = 0
	}

	logs, err := b.db.GetRecentAILogs(5)
	if err != nil {
		return c.Reply(fmt.Sprintf("❌ AI loglarini yuklashda xatolik: %v", err))
	}

	if len(logs) == 0 {
		return c.Reply(fmt.Sprintf("📋 **AI So'rovlar Logi:**\nBugungi jami so'rovlar: %d ta\nHozircha saqlangan loglar mavjud emas.", todayCount))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 **Oxirgi AI So'rovlari (Bugun: %d ta):**\n\n", todayCount))

	for _, l := range logs {
		status := "✅ OK"
		if l.ErrorMessage != "" {
			status = fmt.Sprintf("❌ Xato: %s", l.ErrorMessage)
		}

		promptSnippet := l.Prompt
		if len([]rune(promptSnippet)) > 50 {
			promptSnippet = string([]rune(promptSnippet)[:50]) + "..."
		}
		promptSnippet = strings.ReplaceAll(promptSnippet, "\n", " ")

		respSnippet := l.Response
		if len([]rune(respSnippet)) > 50 {
			respSnippet = string([]rune(respSnippet)[:50]) + "..."
		}
		respSnippet = strings.ReplaceAll(respSnippet, "\n", " ")

		sb.WriteString(fmt.Sprintf("🔹 **#%d** [%s | %s] (%d ms)\n• Holat: %s\n• So'rov: _%s_\n• Javob: _%s_\n• Vaqt: %s\n\n",
			l.ID, l.ActionType, l.ModelName, l.DurationMs, status, promptSnippet, respSnippet, l.CreatedAt.Format("15:04:05")))
	}

	return safeReplyMarkdown(c, sb.String())
}

func (b *Bot) BuildDailyExecutiveReport(ctx context.Context) (string, error) {
	today := time.Now().Format("2006-01-02")
	stats, err := b.db.GetDayStats(today)
	if err != nil {
		return "", fmt.Errorf("statistika olishda xato: %w", err)
	}

	lessonTitle := "Noma'lum"
	state, err := b.db.GetOrCreateGroupState(b.cfg.TelegramGroupID, "")
	if err == nil {
		if l, exists := b.curriculum.GetLesson(state.CurrentLessonID); exists {
			lessonTitle = fmt.Sprintf("#%d: %s", l.ID, l.Title)
		}
	}

	users, _ := b.db.GetTopUsers(20)
	var memberList []string
	for _, u := range users {
		name := u.FirstName
		if u.Username != "" {
			name = fmt.Sprintf("@%s", u.Username)
		}
		memberList = append(memberList, fmt.Sprintf("%s (%d ball)", name, u.Score))
	}

	data := &ai.DailyReportData{
		Date:              today,
		LessonTitle:       lessonTitle,
		TotalChatMessages: stats.TotalChatMessages,
		BotChatMessages:   stats.BotChatMessages,
		UserChatMessages:  stats.UserChatMessages,
		SubmissionsCount:  stats.SubmissionsCount,
		TotalPointsToday:  stats.TotalPointsToday,
		TopLearnerName:    stats.TopLearnerName,
		TopLearnerPoints:  stats.TopLearnerPoints,
		MembersList:       strings.Join(memberList, ", "),
	}

	return b.aiClient.GenerateDailyReport(ctx, data)
}

func (b *Bot) SendDailyReportToAdmin() error {
	if b.teleBot == nil || b.cfg.AdminTelegramID == 0 {
		return nil
	}

	log.Println("📊 Kunlik hisobot generatsiya qilinmoqda va adminga yuborilmoqda...")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	report, err := b.BuildDailyExecutiveReport(ctx)
	if err != nil {
		log.Printf("Kunlik hisobot generatsiyasida xatolik: %v\n", err)
		b.NotifyAdminError("Kunlik hisobot generatsiyasi", err)
		return err
	}

	target := &tele.User{ID: b.cfg.AdminTelegramID}
	header := "📊 **Kunlik Tahliliy Hisobot (Admin uchun)**\n\n"
	fullMsg := header + report

	formatted := MarkdownToTelegramHTML(fullMsg)
	_, sendErr := b.teleBot.Send(target, formatted, &tele.SendOptions{ParseMode: tele.ModeHTML})
	if sendErr != nil {
		log.Printf("Kunlik hisobot HTML send xatosi, oddiy matn yuborilmoqda: %v\n", sendErr)
		_, sendErr = b.teleBot.Send(target, fullMsg)
	}
	return sendErr
}

func (b *Bot) handlePauseToday(c tele.Context) error {
	if b.cfg.AdminTelegramID != 0 && c.Sender().ID != b.cfg.AdminTelegramID {
		return c.Reply("⛔ Bu buyruq faqat bot administratori uchun mo'ljallangan.")
	}

	state, err := b.db.GetOrCreateGroupState(c.Chat().ID, c.Chat().Title)
	if err != nil {
		return c.Reply("Xatolik yuz berdi.")
	}

	today := time.Now().Format("2006-01-02")
	state.PausedDate = today
	_ = b.db.UpdateGroupState(state)

	return c.Reply("🏖️ Bugungi kun dam olish/bayram deb belgilandi. Avtomatik dars va topshiriqlar to'xtatildi. Ertaga reja bo'yicha davom etamiz!")
}

func (b *Bot) handleResumeToday(c tele.Context) error {
	if b.cfg.AdminTelegramID != 0 && c.Sender().ID != b.cfg.AdminTelegramID {
		return c.Reply("⛔ Bu buyruq faqat bot administratori uchun mo'ljallangan.")
	}

	state, err := b.db.GetOrCreateGroupState(c.Chat().ID, c.Chat().Title)
	if err != nil {
		return c.Reply("Xatolik yuz berdi.")
	}

	state.PausedDate = ""
	_ = b.db.UpdateGroupState(state)

	return c.Reply("✅ Darslar jadvali qayta tiklandi!")
}

func (b *Bot) SendWeekendWishToChat(chatID int64) error {
	if b.teleBot == nil {
		return nil
	}

	target := &tele.Chat{ID: chatID}
	msg := `🎉 **Assalomu alaykum, qadrli Go o'rganuvchilar!**

Dam olish kunlaringiz maroqli, sermazmun va fayzli o'tsin! 🏖️
Haftalik o'rganilgan darslardan so'ng miyaga yaxshilab dam bering. Dushanba kuni ertalab soat 10:00 da yangi mavzular va qiziqarli amaliyotlar bilan darslarimizni davom ettiramiz! 🐹🚀`

	_, err := safeSendMarkdownDirect(b.teleBot, target, msg)
	return err
}

