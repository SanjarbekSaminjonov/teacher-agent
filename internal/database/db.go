package database

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
	mu sync.RWMutex
}

func NewDB(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite ochishda xato: %w", err)
	}

	// Optimize SQLite for concurrent usage
	conn.SetMaxOpenConns(1)

	d := &DB{db: conn}
	if err := d.initTables(); err != nil {
		return nil, fmt.Errorf("jadvallarni yaratishda xato: %w", err)
	}

	return d, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) initTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			telegram_id INTEGER PRIMARY KEY,
			username TEXT,
			first_name TEXT,
			score INTEGER DEFAULT 0,
			quizzes_solved INTEGER DEFAULT 0,
			challenges_solved INTEGER DEFAULT 0,
			is_left BOOLEAN DEFAULT 0,
			updated_at TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS group_state (
			chat_id INTEGER PRIMARY KEY,
			group_title TEXT,
			current_lesson_id INTEGER DEFAULT 1,
			lesson_date TEXT DEFAULT '',
			morning_sent BOOLEAN DEFAULT 0,
			afternoon_quiz_sent BOOLEAN DEFAULT 0,
			evening_challenge_sent BOOLEAN DEFAULT 0,
			nudge_sent BOOLEAN DEFAULT 0,
			last_quiz_poll_id TEXT DEFAULT '',
			last_challenge_message_id INTEGER DEFAULT 0,
			paused_date TEXT DEFAULT '',
			weekend_wish_sent BOOLEAN DEFAULT 0,
			deadline_announced BOOLEAN DEFAULT 0,
			icebreaker_sent BOOLEAN DEFAULT 0,
			updated_at TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS quiz_attempts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			telegram_id INTEGER,
			lesson_id INTEGER,
			is_correct BOOLEAN,
			attempted_at TIMESTAMP,
			UNIQUE(telegram_id, lesson_id)
		);`,
		`CREATE TABLE IF NOT EXISTS challenge_submissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			telegram_id INTEGER,
			lesson_id INTEGER,
			code_snippet TEXT,
			ai_feedback TEXT,
			score_awarded INTEGER,
			submitted_at TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS chat_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER,
			sender_name TEXT,
			message_text TEXT,
			is_bot BOOLEAN,
			created_at TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_chat_history ON chat_history(chat_id, created_at DESC);`,
		`CREATE TABLE IF NOT EXISTS user_notes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			telegram_id INTEGER,
			note_type TEXT,
			summary TEXT,
			created_at TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_user_notes ON user_notes(telegram_id);`,
		`CREATE TABLE IF NOT EXISTS ai_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action_type TEXT,
			model_name TEXT,
			prompt TEXT,
			response TEXT,
			duration_ms INTEGER,
			error_message TEXT,
			created_at TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_ai_logs ON ai_logs(created_at DESC);`,
	}

	for _, q := range queries {
		if _, err := d.db.Exec(q); err != nil {
			return err
		}
	}

	// Safe migrations for existing databases
	_, _ = d.db.Exec(`ALTER TABLE group_state ADD COLUMN paused_date TEXT DEFAULT '';`)
	_, _ = d.db.Exec(`ALTER TABLE group_state ADD COLUMN weekend_wish_sent BOOLEAN DEFAULT 0;`)
	_, _ = d.db.Exec(`ALTER TABLE group_state ADD COLUMN deadline_announced BOOLEAN DEFAULT 0;`)
	_, _ = d.db.Exec(`ALTER TABLE group_state ADD COLUMN icebreaker_sent BOOLEAN DEFAULT 0;`)
	_, _ = d.db.Exec(`ALTER TABLE users ADD COLUMN is_left BOOLEAN DEFAULT 0;`)

	return nil
}

func (d *DB) GetOrCreateUser(telegramID int64, username, firstName string) (*User, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	// If placeholder user exists with negative ID for this username, migrate it
	if username != "" && telegramID > 0 {
		var placeholderID int64
		err := d.db.QueryRow(`SELECT telegram_id FROM users WHERE username = ? AND telegram_id < 0`, username).Scan(&placeholderID)
		if err == nil && placeholderID != 0 {
			_, _ = d.db.Exec(`UPDATE users SET telegram_id = ? WHERE telegram_id = ?`, telegramID, placeholderID)
			_, _ = d.db.Exec(`UPDATE user_notes SET telegram_id = ? WHERE telegram_id = ?`, telegramID, placeholderID)
		}
	}

	_, err := d.db.Exec(`
		INSERT INTO users (telegram_id, username, first_name, score, quizzes_solved, challenges_solved, updated_at)
		VALUES (?, ?, ?, 0, 0, 0, ?)
		ON CONFLICT(telegram_id) DO UPDATE SET
			username = excluded.username,
			first_name = excluded.first_name,
			is_left = 0,
			updated_at = excluded.updated_at;
	`, telegramID, username, firstName, now)
	if err != nil {
		return nil, err
	}

	return d.getUserUnsafe(telegramID)
}

func (d *DB) MarkUserLeft(telegramID int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`
		UPDATE users
		SET is_left = 1,
		    updated_at = ?
		WHERE telegram_id = ?
	`, time.Now(), telegramID)
	return err
}

func (d *DB) getUserUnsafe(telegramID int64) (*User, error) {
	var u User
	err := d.db.QueryRow(`
		SELECT telegram_id, username, first_name, score, quizzes_solved, challenges_solved, is_left, updated_at
		FROM users WHERE telegram_id = ?
	`, telegramID).Scan(&u.TelegramID, &u.Username, &u.FirstName, &u.Score, &u.QuizzesSolved, &u.ChallengesSolved, &u.IsLeft, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *DB) GetUser(telegramID int64) (*User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.getUserUnsafe(telegramID)
}

func (d *DB) AddScore(telegramID int64, points int, isQuiz bool, isChallenge bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	quizInc := 0
	if isQuiz {
		quizInc = 1
	}
	challengeInc := 0
	if isChallenge {
		challengeInc = 1
	}

	_, err := d.db.Exec(`
		UPDATE users
		SET score = score + ?,
		    quizzes_solved = quizzes_solved + ?,
		    challenges_solved = challenges_solved + ?,
		    is_left = 0,
		    updated_at = ?
		WHERE telegram_id = ?
	`, points, quizInc, challengeInc, time.Now(), telegramID)
	return err
}

func (d *DB) GetTopUsers(limit int) ([]User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.Query(`
		SELECT telegram_id, username, first_name, score, quizzes_solved, challenges_solved, is_left, updated_at
		FROM users
		WHERE is_left = 0
		ORDER BY score DESC, quizzes_solved DESC, challenges_solved DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.TelegramID, &u.Username, &u.FirstName, &u.Score, &u.QuizzesSolved, &u.ChallengesSolved, &u.IsLeft, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (d *DB) GetInactiveUsers(excludeTelegramID int64, limit int) ([]User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.Query(`
		SELECT telegram_id, username, first_name, score, quizzes_solved, challenges_solved, is_left, updated_at
		FROM users
		WHERE telegram_id != ? AND is_left = 0 AND username != ''
		ORDER BY quizzes_solved ASC, challenges_solved ASC, updated_at ASC
		LIMIT ?
	`, excludeTelegramID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.TelegramID, &u.Username, &u.FirstName, &u.Score, &u.QuizzesSolved, &u.ChallengesSolved, &u.IsLeft, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (d *DB) GetUnsubmittedUsersForToday(lessonID int, excludeTelegramID int64, limit int) ([]User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	today := time.Now().Format("2006-01-02")
	rows, err := d.db.Query(`
		SELECT u.telegram_id, u.username, u.first_name, u.score, u.quizzes_solved, u.challenges_solved, u.is_left, u.updated_at
		FROM users u
		WHERE u.telegram_id != ? AND u.username != '' AND u.is_left = 0
		  AND u.telegram_id NOT IN (
			SELECT telegram_id FROM challenge_submissions
			WHERE lesson_id = ? AND substr(submitted_at, 1, 10) = ?
		  )
		ORDER BY u.score ASC, u.updated_at ASC
		LIMIT ?
	`, excludeTelegramID, lessonID, today, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.TelegramID, &u.Username, &u.FirstName, &u.Score, &u.QuizzesSolved, &u.ChallengesSolved, &u.IsLeft, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (d *DB) GetOrCreateGroupState(chatID int64, title string) (*GroupState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	state, err := d.getGroupStateUnsafe(chatID)
	if err == nil {
		return state, nil
	}

	// If chatID is not found, check if there's an existing group_state (e.g. from before supergroup migration)
	var existingOldID int64
	checkErr := d.db.QueryRow(`SELECT chat_id FROM group_state WHERE chat_id != ? LIMIT 1`, chatID).Scan(&existingOldID)
	if checkErr == nil && existingOldID != 0 {
		_ = d.migrateGroupStateUnsafe(existingOldID, chatID)
		if migrated, mErr := d.getGroupStateUnsafe(chatID); mErr == nil {
			return migrated, nil
		}
	}

	now := time.Now()
	newState := GroupState{
		ChatID:            chatID,
		GroupTitle:        title,
		CurrentLessonID:   1,
		LessonDate:        "",
		PausedDate:        "",
		WeekendWishSent:   false,
		DeadlineAnnounced: false,
		IcebreakerSent:    false,
		UpdatedAt:         now,
	}
	_, insertErr := d.db.Exec(`
		INSERT INTO group_state (chat_id, group_title, current_lesson_id, lesson_date, paused_date, weekend_wish_sent, deadline_announced, icebreaker_sent, updated_at)
		VALUES (?, ?, 1, '', '', 0, 0, 0, ?)
	`, chatID, title, now)
	if insertErr != nil {
		return nil, insertErr
	}
	return &newState, nil
}

func (d *DB) getGroupStateUnsafe(chatID int64) (*GroupState, error) {
	var state GroupState
	err := d.db.QueryRow(`
		SELECT chat_id, group_title, current_lesson_id, lesson_date, morning_sent,
		       afternoon_quiz_sent, evening_challenge_sent, nudge_sent,
		       last_quiz_poll_id, last_challenge_message_id, paused_date, weekend_wish_sent, deadline_announced, icebreaker_sent, updated_at
		FROM group_state WHERE chat_id = ?
	`, chatID).Scan(
		&state.ChatID, &state.GroupTitle, &state.CurrentLessonID, &state.LessonDate,
		&state.MorningSent, &state.AfternoonQuizSent, &state.EveningChallengeSent,
		&state.NudgeSent, &state.LastQuizPollID, &state.LastChallengeMessageID,
		&state.PausedDate, &state.WeekendWishSent, &state.DeadlineAnnounced, &state.IcebreakerSent, &state.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (d *DB) MigrateGroupState(oldChatID, newChatID int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.migrateGroupStateUnsafe(oldChatID, newChatID)
}

func (d *DB) migrateGroupStateUnsafe(oldChatID, newChatID int64) error {
	if oldChatID == 0 || newChatID == 0 || oldChatID == newChatID {
		return nil
	}

	var oldState GroupState
	err := d.db.QueryRow(`
		SELECT chat_id, group_title, current_lesson_id, lesson_date, morning_sent,
		       afternoon_quiz_sent, evening_challenge_sent, nudge_sent,
		       last_quiz_poll_id, last_challenge_message_id, paused_date, weekend_wish_sent, deadline_announced, icebreaker_sent
		FROM group_state WHERE chat_id = ?
	`, oldChatID).Scan(
		&oldState.ChatID, &oldState.GroupTitle, &oldState.CurrentLessonID, &oldState.LessonDate,
		&oldState.MorningSent, &oldState.AfternoonQuizSent, &oldState.EveningChallengeSent,
		&oldState.NudgeSent, &oldState.LastQuizPollID, &oldState.LastChallengeMessageID,
		&oldState.PausedDate, &oldState.WeekendWishSent, &oldState.DeadlineAnnounced, &oldState.IcebreakerSent,
	)
	if err != nil {
		return err
	}

	// Update newChatID if it exists, otherwise insert
	_, err = d.db.Exec(`
		INSERT INTO group_state (chat_id, group_title, current_lesson_id, lesson_date, morning_sent,
		                         afternoon_quiz_sent, evening_challenge_sent, nudge_sent,
		                         last_quiz_poll_id, last_challenge_message_id, paused_date,
		                         weekend_wish_sent, deadline_announced, icebreaker_sent, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
			group_title = excluded.group_title,
			current_lesson_id = excluded.current_lesson_id,
			lesson_date = excluded.lesson_date,
			morning_sent = excluded.morning_sent,
			afternoon_quiz_sent = excluded.afternoon_quiz_sent,
			evening_challenge_sent = excluded.evening_challenge_sent,
			nudge_sent = excluded.nudge_sent,
			last_quiz_poll_id = excluded.last_quiz_poll_id,
			last_challenge_message_id = excluded.last_challenge_message_id,
			paused_date = excluded.paused_date,
			weekend_wish_sent = excluded.weekend_wish_sent,
			deadline_announced = excluded.deadline_announced,
			icebreaker_sent = excluded.icebreaker_sent,
			updated_at = excluded.updated_at
	`, newChatID, oldState.GroupTitle, oldState.CurrentLessonID, oldState.LessonDate,
		oldState.MorningSent, oldState.AfternoonQuizSent, oldState.EveningChallengeSent,
		oldState.NudgeSent, oldState.LastQuizPollID, oldState.LastChallengeMessageID,
		oldState.PausedDate, oldState.WeekendWishSent, oldState.DeadlineAnnounced, oldState.IcebreakerSent, time.Now())

	if err != nil {
		return err
	}

	// Delete oldChatID from group_state
	_, _ = d.db.Exec(`DELETE FROM group_state WHERE chat_id = ?`, oldChatID)

	// Update chat_history chat_id from old to new
	_, _ = d.db.Exec(`UPDATE chat_history SET chat_id = ? WHERE chat_id = ?`, newChatID, oldChatID)

	return nil
}

func (d *DB) GetAllGroupStates() ([]GroupState, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.Query(`
		SELECT chat_id, group_title, current_lesson_id, lesson_date, morning_sent,
		       afternoon_quiz_sent, evening_challenge_sent, nudge_sent,
		       last_quiz_poll_id, last_challenge_message_id, paused_date, weekend_wish_sent, deadline_announced, icebreaker_sent, updated_at
		FROM group_state
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []GroupState
	for rows.Next() {
		var s GroupState
		if err := rows.Scan(
			&s.ChatID, &s.GroupTitle, &s.CurrentLessonID, &s.LessonDate,
			&s.MorningSent, &s.AfternoonQuizSent, &s.EveningChallengeSent,
			&s.NudgeSent, &s.LastQuizPollID, &s.LastChallengeMessageID,
			&s.PausedDate, &s.WeekendWishSent, &s.DeadlineAnnounced, &s.IcebreakerSent, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		states = append(states, s)
	}
	return states, nil
}

func (d *DB) UpdateGroupState(s *GroupState) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	s.UpdatedAt = time.Now()
	_, err := d.db.Exec(`
		UPDATE group_state
		SET group_title = ?,
		    current_lesson_id = ?,
		    lesson_date = ?,
		    morning_sent = ?,
		    afternoon_quiz_sent = ?,
		    evening_challenge_sent = ?,
		    nudge_sent = ?,
		    last_quiz_poll_id = ?,
		    last_challenge_message_id = ?,
		    paused_date = ?,
		    weekend_wish_sent = ?,
		    deadline_announced = ?,
		    icebreaker_sent = ?,
		    updated_at = ?
		WHERE chat_id = ?
	`, s.GroupTitle, s.CurrentLessonID, s.LessonDate, s.MorningSent,
		s.AfternoonQuizSent, s.EveningChallengeSent, s.NudgeSent,
		s.LastQuizPollID, s.LastChallengeMessageID, s.PausedDate, s.WeekendWishSent, s.DeadlineAnnounced, s.IcebreakerSent, s.UpdatedAt, s.ChatID)
	return err
}

func (d *DB) RecordQuizAttempt(telegramID int64, lessonID int, isCorrect bool) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	res, err := d.db.Exec(`
		INSERT OR IGNORE INTO quiz_attempts (telegram_id, lesson_id, is_correct, attempted_at)
		VALUES (?, ?, ?, ?)
	`, telegramID, lessonID, isCorrect, time.Now())
	if err != nil {
		return false, err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	// Return true if this was the first attempt
	return rows > 0, nil
}

func (d *DB) RecordSubmission(telegramID int64, lessonID int, code, feedback string, score int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`
		INSERT INTO challenge_submissions (telegram_id, lesson_id, code_snippet, ai_feedback, score_awarded, submitted_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, telegramID, lessonID, code, feedback, score, time.Now())
	return err
}

func (d *DB) CountSubmissionsForLesson(lessonID int, since time.Time) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var count int
	err := d.db.QueryRow(`
		SELECT COUNT(DISTINCT telegram_id)
		FROM challenge_submissions
		WHERE lesson_id = ? AND submitted_at >= ?
	`, lessonID, since).Scan(&count)
	return count, err
}

func (d *DB) SaveChatMessage(chatID int64, senderName, text string, isBot bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`
		INSERT INTO chat_history (chat_id, sender_name, message_text, is_bot, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, chatID, senderName, text, isBot, time.Now())
	return err
}

func (d *DB) GetRecentChatHistory(chatID int64, limit int) ([]ChatMessage, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.Query(`
		SELECT id, chat_id, sender_name, message_text, is_bot, created_at
		FROM (
			SELECT id, chat_id, sender_name, message_text, is_bot, created_at
			FROM chat_history
			WHERE chat_id = ?
			ORDER BY id DESC
			LIMIT ?
		)
		ORDER BY id ASC;
	`, chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.ChatID, &m.SenderName, &m.MessageText, &m.IsBot, &m.CreatedAt); err != nil {
			return nil, err
		}
		history = append(history, m)
	}

	return history, nil
}

func (d *DB) SaveUserNote(telegramID int64, noteType, summary string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`
		INSERT INTO user_notes (telegram_id, note_type, summary, created_at)
		VALUES (?, ?, ?, ?)
	`, telegramID, noteType, summary, time.Now())
	return err
}

func (d *DB) GetLatestUserNote(telegramID int64) (*UserNote, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var n UserNote
	err := d.db.QueryRow(`
		SELECT id, telegram_id, note_type, summary, created_at
		FROM user_notes
		WHERE telegram_id = ?
		ORDER BY id DESC
		LIMIT 1
	`, telegramID).Scan(&n.ID, &n.TelegramID, &n.NoteType, &n.Summary, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (d *DB) GetRandomUserNote(telegramID int64) (*UserNote, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var n UserNote
	err := d.db.QueryRow(`
		SELECT id, telegram_id, note_type, summary, created_at
		FROM user_notes
		WHERE telegram_id = ?
		ORDER BY RANDOM()
		LIMIT 1
	`, telegramID).Scan(&n.ID, &n.TelegramID, &n.NoteType, &n.Summary, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (d *DB) SaveAILog(actionType, modelName, prompt, response string, durationMs int64, errMsg string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`
		INSERT INTO ai_logs (action_type, model_name, prompt, response, duration_ms, error_message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, actionType, modelName, prompt, response, durationMs, errMsg, time.Now())
	return err
}

func (d *DB) GetRecentAILogs(limit int) ([]AILog, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.Query(`
		SELECT id, action_type, model_name, prompt, response, duration_ms, COALESCE(error_message, ''), created_at
		FROM ai_logs
		ORDER BY id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []AILog
	for rows.Next() {
		var l AILog
		if err := rows.Scan(&l.ID, &l.ActionType, &l.ModelName, &l.Prompt, &l.Response, &l.DurationMs, &l.ErrorMessage, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, nil
}

func (d *DB) GetTodayAICount() (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	today := time.Now().Format("2006-01-02")
	var count int
	err := d.db.QueryRow(`
		SELECT COUNT(*)
		FROM ai_logs
		WHERE substr(created_at, 1, 10) = ?
	`, today).Scan(&count)
	return count, err
}

func (d *DB) GetDayStats(dateStr string) (*DayStats, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := &DayStats{Date: dateStr}

	// 1. Total chat messages today
	_ = d.db.QueryRow(`
		SELECT COUNT(*), 
		       COALESCE(SUM(CASE WHEN is_bot = 1 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN is_bot = 0 THEN 1 ELSE 0 END), 0)
		FROM chat_history
		WHERE substr(created_at, 1, 10) = ?
	`, dateStr).Scan(&stats.TotalChatMessages, &stats.BotChatMessages, &stats.UserChatMessages)

	// 2. Submissions count (distinct learners) & total points today (sum of best scores)
	_ = d.db.QueryRow(`
		SELECT COUNT(DISTINCT telegram_id),
		       COALESCE(SUM(user_lesson_score), 0)
		FROM (
			SELECT telegram_id, lesson_id, MAX(score_awarded) as user_lesson_score
			FROM challenge_submissions
			WHERE substr(submitted_at, 1, 10) = ?
			GROUP BY telegram_id, lesson_id
		)
	`, dateStr).Scan(&stats.SubmissionsCount, &stats.TotalPointsToday)

	// 3. Active users count today
	_ = d.db.QueryRow(`
		SELECT COUNT(DISTINCT telegram_id)
		FROM users
		WHERE substr(updated_at, 1, 10) = ?
	`, dateStr).Scan(&stats.UsersActive)

	// 4. Top learner today
	var topID int64
	var topScore int
	err := d.db.QueryRow(`
		SELECT telegram_id, SUM(user_lesson_score) as total_user_score
		FROM (
			SELECT telegram_id, lesson_id, MAX(score_awarded) as user_lesson_score
			FROM challenge_submissions
			WHERE substr(submitted_at, 1, 10) = ?
			GROUP BY telegram_id, lesson_id
		)
		GROUP BY telegram_id
		ORDER BY total_user_score DESC
		LIMIT 1
	`, dateStr).Scan(&topID, &topScore)
	if err == nil && topID != 0 {
		stats.TopLearnerPoints = topScore
		var name string
		if err := d.db.QueryRow(`SELECT COALESCE(NULLIF(first_name, ''), username) FROM users WHERE telegram_id = ?`, topID).Scan(&name); err == nil {
			stats.TopLearnerName = name
		}
	}

	return stats, nil
}

func (d *DB) GetStudyContext(chatID int64, lessonID int, lessonTitle, prevLessonTitle, officialChallengeTitle, officialChallengeTask string, morningSent, quizSent, challengeSent, deadlineAnnounced bool, currentUserID int64, currentUserName string) (*StudyContext, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	now := time.Now()
	today := now.Format("2006-01-02")
	timeStr := now.Format("15:04")

	ctx := &StudyContext{
		CurrentDate:            today,
		CurrentTime:            timeStr,
		LessonID:               lessonID,
		LessonTitle:            lessonTitle,
		PreviousLessonTitle:    prevLessonTitle,
		MorningSent:            morningSent,
		QuizSent:               quizSent,
		OfficialChallengeTitle: officialChallengeTitle,
		OfficialChallengeTask:  officialChallengeTask,
		ChallengeSent:          challengeSent,
		DeadlineAnnounced:      deadlineAnnounced,
		CurrentUserName:        currentUserName,
		Submitters:             make([]SubmitterInfo, 0),
		UnsubmittedNames:       make([]string, 0),
	}

	// 1. Submissions for this lesson today (GROUP BY telegram_id to prevent duplicates)
	rows, err := d.db.Query(`
		SELECT cs.telegram_id, COALESCE(u.first_name, ''), COALESCE(u.username, ''), MAX(cs.score_awarded)
		FROM challenge_submissions cs
		LEFT JOIN users u ON cs.telegram_id = u.telegram_id
		WHERE cs.lesson_id = ? AND substr(cs.submitted_at, 1, 10) = ?
		GROUP BY cs.telegram_id
		ORDER BY MAX(cs.score_awarded) DESC
	`, lessonID, today)
	if err != nil {
		return ctx, err
	}
	defer rows.Close()

	submittedMap := make(map[int64]bool)
	for rows.Next() {
		var sub SubmitterInfo
		if err := rows.Scan(&sub.TelegramID, &sub.Name, &sub.Username, &sub.ScoreAwarded); err == nil {
			if sub.Name == "" {
				sub.Name = sub.Username
			}
			ctx.Submitters = append(ctx.Submitters, sub)
			submittedMap[sub.TelegramID] = true
			if sub.TelegramID == currentUserID {
				ctx.UserHasSubmitted = true
			}
		}
	}
	ctx.TotalSubmissions = len(ctx.Submitters)

	// 2. Fetch current user score and rank, plus top leaderboard summary
	topUsers, err := d.GetTopUsers(5)
	if err == nil && len(topUsers) > 0 {
		var lbLines []string
		for i, u := range topUsers {
			name := u.FirstName
			if u.Username != "" {
				name = "@" + u.Username
			}
			lbLines = append(lbLines, fmt.Sprintf("%d. %s: %d ball", i+1, name, u.Score))
			if u.TelegramID == currentUserID {
				ctx.CurrentUserScore = u.Score
				ctx.CurrentUserRank = i + 1
			}
		}
		ctx.LeaderboardSummary = strings.Join(lbLines, "\n")
	}

	if ctx.CurrentUserRank == 0 && currentUserID != 0 {
		if u, err := d.GetUser(currentUserID); err == nil && u != nil {
			ctx.CurrentUserScore = u.Score
		}
	}

	return ctx, nil
}

func FormatStudyContext(sc *StudyContext) string {
	if sc == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("• BUGUNGI SANA VA VAQT: %s, soat %s\n", sc.CurrentDate, sc.CurrentTime))
	sb.WriteString(fmt.Sprintf("• JORIY KURS BOSQICHI: #%d-dars kuni.\n", sc.LessonID))

	if !sc.MorningSent {
		sb.WriteString(fmt.Sprintf("• MUHIM JADVAL HOLATI (Ertalabki holat): Bugungi yangi dars (#%d: \"%s\") hali guruhga yuborilmagan! Jadvalimizga ko'ra, ertalabki dars soat 10:00 da avtomatik e'lon qilinadi.\n", sc.LessonID, sc.LessonTitle))
		if sc.PreviousLessonTitle != "" {
			sb.WriteString(fmt.Sprintf("• HOZIRGI FAOL VAQT: Kechagi darsni takrorlash vaqti! Kecha o'tilgan dars: \"%s\". Agar o'quvchilar hozir savol berishsa, kechagi mavzu bo'yicha javob bering yoki yangi dars soat 10:00 da boshlanishini ayting.\n", sc.PreviousLessonTitle))
		} else {
			sb.WriteString("• HOZIRGI FAOL VAQT: Yangi dars soat 10:00 da boshlanadi.\n")
		}
		sb.WriteString(fmt.Sprintf("• QAT'IY QOIDA: Yangi dars (#%d: \"%s\") hali boshlanmagani sababli, uning mazmuni va topshirig'ini SOAT 10:00 GACHA SIR TUTING! O'zingizdan oldinlab yangi mavzuni boshladik deb gapirmang!\n", sc.LessonID, sc.LessonTitle))
	} else {
		sb.WriteString(fmt.Sprintf("• BUGUNGI MAVZU: #%d: \"%s\" (Guruhga 10:00 da e'lon qilingan va faol o'rganilmoqda).\n", sc.LessonID, sc.LessonTitle))
	}

	if sc.LeaderboardSummary != "" {
		sb.WriteString("• Rasmiy Reyting Jadvali (Bazadagi aniq ballar):\n")
		sb.WriteString(sc.LeaderboardSummary)
		sb.WriteString("\n(ESLATMA: Agar kimdir ballar yoki reyting haqida so'rasa, faqat mana shu aniq raqamlarni ayting!)\n")
	}

	if sc.ChallengeSent {
		sb.WriteString("• Kechki kod topshirig'i (Challenge): Soat 15:00 da guruhga yuborilgan (FAOL!).\n")
		if sc.DeadlineAnnounced {
			sb.WriteString("• DIQQAT: Soat 17:30 da bugungi topshiriq muddati (deadline) yakunlangan! Endi kod yuborganlarga tahlil beriladi, lekin reyting balli berilmaydi.\n")
		}
		if sc.OfficialChallengeTask != "" {
			sb.WriteString(fmt.Sprintf("• RASMIY AMALIY TOPSHIRIQ (CHALLENGE):\nSarlavha: \"%s\"\nAniq shart:\n\"\"\"\n%s\n\"\"\"\n(Agar o'quvchi topshiriq shartini so'rasa, aynan mana shu rasmiy shartni bering!)\n",
				sc.OfficialChallengeTitle, sc.OfficialChallengeTask))
		}

		if sc.TotalSubmissions == 0 {
			sb.WriteString("• Topshiriqni bajarganlar: Hozircha hech kim topshirmagan (0 kishi).\n")
		} else {
			var names []string
			for _, s := range sc.Submitters {
				displayName := s.Name
				if s.Username != "" {
					displayName = "@" + s.Username
				}
				names = append(names, fmt.Sprintf("%s (%d ball)", displayName, s.ScoreAwarded))
			}
			sb.WriteString(fmt.Sprintf("• Topshiriqni muvaffaqiyatli topshirganlar (%d nafar): %s\n", sc.TotalSubmissions, strings.Join(names, ", ")))
		}

		if sc.CurrentUserName != "" {
			if sc.UserHasSubmitted {
				sb.WriteString(fmt.Sprintf("• Hozir gapirayotgan o'quvchi (%s): Topshiriqni topshirgan ✅ (Jami bali: %d ball)\n", sc.CurrentUserName, sc.CurrentUserScore))
			} else {
				sb.WriteString(fmt.Sprintf("• Hozir gapirayotgan o'quvchi (%s): Topshiriqni HALI TOPSHIRMAGAN ❌ (Jami bali: %d ball)\n", sc.CurrentUserName, sc.CurrentUserScore))
			}
		}
	} else {
		sb.WriteString("• Kechki kod topshirig'i (Challenge): Hali guruhga yuborilmagan! U bugun soat 15:00 da e'lon qilinadi.\n")
		sb.WriteString("• QAT'IY QOIDA: Agar o'quvchi 'topshiriq qani?', 'topshiriq shartini ber' desa, HECH QACHON topshiriq shartini oldindan oshkor qilmang! 'Bugungi amaliy kod topshirig'i reja bo'yicha soat 15:00 da e'lon qilinadi, ungacha nazariyani o'rganib turing' deb javob bering!\n")
	}

	return sb.String()
}

func (d *DB) GetMaxScoreForLesson(telegramID int64, lessonID int) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var maxScore sql.NullInt64
	err := d.db.QueryRow(`
		SELECT MAX(score_awarded)
		FROM challenge_submissions
		WHERE telegram_id = ? AND lesson_id = ?
	`, telegramID, lessonID).Scan(&maxScore)
	if err != nil || !maxScore.Valid {
		return 0, err
	}
	return int(maxScore.Int64), nil
}

