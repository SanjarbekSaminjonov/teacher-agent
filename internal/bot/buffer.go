package bot

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"teacher-agent/internal/database"

	tele "gopkg.in/telebot.v3"
)

type BufferedEvent struct {
	EventSummary string
	FallbackText string
	CreatedAt    time.Time
}

type ChatDebouncer struct {
	mu          sync.Mutex
	events      map[int64][]BufferedEvent
	timers      map[int64]*time.Timer
	b           *Bot
	quietWindow time.Duration
}

func NewChatDebouncer(b *Bot, quietWindow time.Duration) *ChatDebouncer {
	return &ChatDebouncer{
		events:      make(map[int64][]BufferedEvent),
		timers:      make(map[int64]*time.Timer),
		b:           b,
		quietWindow: quietWindow,
	}
}

func (cd *ChatDebouncer) HasActiveEvents(chatID int64) bool {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	return len(cd.events[chatID]) > 0
}

func (cd *ChatDebouncer) AddEvent(chatID int64, eventSummary, fallbackText string) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	cd.events[chatID] = append(cd.events[chatID], BufferedEvent{
		EventSummary: eventSummary,
		FallbackText: fallbackText,
		CreatedAt:    time.Now(),
	})

	// Reset timer for this chat
	if t, exists := cd.timers[chatID]; exists {
		t.Stop()
	}

	cd.timers[chatID] = time.AfterFunc(cd.quietWindow, func() {
		cd.flushEvents(chatID)
	})
}

func (cd *ChatDebouncer) flushEvents(chatID int64) {
	cd.mu.Lock()
	events := cd.events[chatID]
	delete(cd.events, chatID)
	delete(cd.timers, chatID)
	cd.mu.Unlock()

	if len(events) == 0 || cd.b == nil || cd.b.teleBot == nil {
		return
	}

	// On weekends or when group is paused for holiday, don't interrupt casual chatter
	weekday := time.Now().Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return
	}

	state, err := cd.b.db.GetOrCreateGroupState(chatID, "")
	today := time.Now().Format("2006-01-02")
	if err == nil && state.PausedDate == today {
		return
	}

	target := &tele.Chat{ID: chatID}

	// 1. If only 1 event, e.g. a single user joined
	if len(events) == 1 {
		if events[0].FallbackText != "" {
			_ = safeSendMarkdownDirect(cd.b.teleBot, target, events[0].FallbackText)
		}
		return
	}

	// 2. Multiple events: Gemini AI batches and summarizes!
	var eventStrings []string
	for _, e := range events {
		eventStrings = append(eventStrings, e.EventSummary)
	}

	studyCtxStr := ""
	lessonTitle := "Go Dasturlash Tili"
	if err == nil {
		if lesson, ok := cd.b.curriculum.GetLesson(state.CurrentLessonID); ok {
			lessonTitle = lesson.Title
			sc, err := cd.b.db.GetStudyContext(chatID, lesson.ID, lesson.Title, lesson.Challenge.Title, lesson.Challenge.Task, state.EveningChallengeSent, 0, "")
			if err == nil {
				studyCtxStr = database.FormatStudyContext(sc)
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	summary, err := cd.b.aiClient.GenerateBatchSummary(ctx, eventStrings, lessonTitle, studyCtxStr)
	if err != nil || strings.TrimSpace(summary) == "" {
		log.Printf("Batch summary generatsiyasida xatolik: %v\n", err)
		for _, e := range events {
			if e.FallbackText != "" {
				_ = safeSendMarkdownDirect(cd.b.teleBot, target, e.FallbackText)
			}
		}
		return
	}

	_ = safeSendMarkdownDirect(cd.b.teleBot, target, summary)
}

func safeSendMarkdownDirect(b *tele.Bot, target tele.Recipient, text string) error {
	if b == nil {
		return nil
	}
	formatted := MarkdownToTelegramHTML(text)
	_, err := b.Send(target, formatted, &tele.SendOptions{ParseMode: tele.ModeHTML})
	if err != nil {
		log.Printf("HTML send direct xatosi, oddiy matn yuborilmoqda: %v\n", err)
		_, err = b.Send(target, text)
	}
	return err
}
