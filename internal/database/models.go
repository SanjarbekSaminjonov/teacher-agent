package database

import "time"

type User struct {
	TelegramID       int64     `json:"telegram_id"`
	Username         string    `json:"username"`
	FirstName        string    `json:"first_name"`
	Score            int       `json:"score"`
	QuizzesSolved    int       `json:"quizzes_solved"`
	ChallengesSolved int       `json:"challenges_solved"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type GroupState struct {
	ChatID                  int64     `json:"chat_id"`
	GroupTitle              string    `json:"group_title"`
	CurrentLessonID         int       `json:"current_lesson_id"`
	LessonDate              string    `json:"lesson_date"`
	MorningSent             bool      `json:"morning_sent"`
	AfternoonQuizSent       bool      `json:"afternoon_quiz_sent"`
	EveningChallengeSent    bool      `json:"evening_challenge_sent"`
	NudgeSent               bool      `json:"nudge_sent"`
	LastQuizPollID          string    `json:"last_quiz_poll_id"`
	LastChallengeMessageID  int       `json:"last_challenge_message_id"`
	PausedDate              string    `json:"paused_date"`
	WeekendWishSent         bool      `json:"weekend_wish_sent"`
	DeadlineAnnounced       bool      `json:"deadline_announced"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type Submission struct {
	ID           int64     `json:"id"`
	TelegramID   int64     `json:"telegram_id"`
	LessonID     int       `json:"lesson_id"`
	CodeSnippet  string    `json:"code_snippet"`
	AIFeedback   string    `json:"ai_feedback"`
	ScoreAwarded int       `json:"score_awarded"`
	SubmittedAt  time.Time `json:"submitted_at"`
}

type ChatMessage struct {
	ID          int64     `json:"id"`
	ChatID      int64     `json:"chat_id"`
	SenderName  string    `json:"sender_name"`
	MessageText string    `json:"message_text"`
	IsBot       bool      `json:"is_bot"`
	CreatedAt   time.Time `json:"created_at"`
}

type UserNote struct {
	ID         int64     `json:"id"`
	TelegramID int64     `json:"telegram_id"`
	NoteType   string    `json:"note_type"`
	Summary    string    `json:"summary"`
	CreatedAt  time.Time `json:"created_at"`
}

type AILog struct {
	ID           int64     `json:"id"`
	ActionType   string    `json:"action_type"`
	ModelName    string    `json:"model_name"`
	Prompt       string    `json:"prompt"`
	Response     string    `json:"response"`
	DurationMs   int64     `json:"duration_ms"`
	ErrorMessage string    `json:"error_message"`
	CreatedAt    time.Time `json:"created_at"`
}

type DayStats struct {
	Date              string `json:"date"`
	TotalChatMessages int    `json:"total_chat_messages"`
	BotChatMessages   int    `json:"bot_chat_messages"`
	UserChatMessages  int    `json:"user_chat_messages"`
	SubmissionsCount  int    `json:"submissions_count"`
	TotalPointsToday  int    `json:"total_points_today"`
	UsersActive       int    `json:"users_active"`
	TopLearnerName    string `json:"top_learner_name"`
	TopLearnerPoints  int    `json:"top_learner_points"`
}

type SubmitterInfo struct {
	TelegramID   int64  `json:"telegram_id"`
	Name         string `json:"name"`
	Username     string `json:"username"`
	ScoreAwarded int    `json:"score_awarded"`
}

type StudyContext struct {
	LessonID               int             `json:"lesson_id"`
	LessonTitle            string          `json:"lesson_title"`
	OfficialChallengeTitle string          `json:"official_challenge_title"`
	OfficialChallengeTask  string          `json:"official_challenge_task"`
	ChallengeSent          bool            `json:"challenge_sent"`
	TotalSubmissions       int             `json:"total_submissions"`
	Submitters             []SubmitterInfo `json:"submitters"`
	UnsubmittedNames       []string        `json:"unsubmitted_names"`
	UserHasSubmitted       bool            `json:"user_has_submitted"`
	CurrentUserName        string          `json:"current_user_name"`
	CurrentUserScore       int             `json:"current_user_score"`
	CurrentUserRank        int             `json:"current_user_rank"`
	LeaderboardSummary     string          `json:"leaderboard_summary"`
}

