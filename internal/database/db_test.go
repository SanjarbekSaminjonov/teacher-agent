package database_test

import (
	"os"
	"testing"
	"time"

	"learn-go-bot/internal/database"
)

func TestDatabaseOperations(t *testing.T) {
	testDBPath := "./test_bot.db"
	defer os.Remove(testDBPath)

	db, err := database.NewDB(testDBPath)
	if err != nil {
		t.Fatalf("Test DB ochishda xato: %v", err)
	}
	defer db.Close()

	// 1. Test User Creation
	user, err := db.GetOrCreateUser(12345, "alisher", "Alisher")
	if err != nil {
		t.Fatalf("User yaratishda xato: %v", err)
	}
	if user.TelegramID != 12345 || user.FirstName != "Alisher" {
		t.Errorf("User ma'lumotlari kutilgandek emas: %+v", user)
	}

	// 2. Test AddScore
	if err := db.AddScore(12345, 10, true, false); err != nil {
		t.Fatalf("Score qo'shishda xato: %v", err)
	}

	u, err := db.GetUser(12345)
	if err != nil {
		t.Fatalf("Userni olishda xato: %v", err)
	}
	if u.Score != 10 || u.QuizzesSolved != 1 {
		t.Errorf("Kutilgan score: 10, quizzes: 1, olingan: score=%d, quizzes=%d", u.Score, u.QuizzesSolved)
	}

	// 3. Test Quiz Attempt (unique attempt prevention)
	firstAttempt, err := db.RecordQuizAttempt(12345, 1, true)
	if err != nil {
		t.Fatalf("Quiz attempt yozishda xato: %v", err)
	}
	if !firstAttempt {
		t.Errorf("Birinchi attempt true bo'lishi kerak edi")
	}

	// Second attempt should return false
	secondAttempt, err := db.RecordQuizAttempt(12345, 1, false)
	if err != nil {
		t.Fatalf("Ikkinchi attemptda xato: %v", err)
	}
	if secondAttempt {
		t.Errorf("Ikkinchi attempt false bo'lishi kerak edi (takrorlanish)")
	}

	// 4. Test Challenge Submission
	if err := db.RecordSubmission(12345, 1, "fmt.Println(\"Hello\")", "Ajoyib!", 15); err != nil {
		t.Fatalf("Submission yozishda xato: %v", err)
	}

	count, err := db.CountSubmissionsForLesson(1, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("Submission hisoblashda xato: %v", err)
	}
	if count != 1 {
		t.Errorf("Kutilgan submission soni: 1, lekin %d ta", count)
	}

	// 5. Test GroupState
	state, err := db.GetOrCreateGroupState(-100999, "Test Go Guruhi")
	if err != nil {
		t.Fatalf("GroupState yaratishda xato: %v", err)
	}
	if state.CurrentLessonID != 1 {
		t.Errorf("Boshlang'ich dars 1 bo'lishi kerak edi, olingan: %d", state.CurrentLessonID)
	}

	state.CurrentLessonID = 2
	state.MorningSent = true
	if err := db.UpdateGroupState(state); err != nil {
		t.Fatalf("GroupState yangilashda xato: %v", err)
	}

	updatedState, err := db.GetOrCreateGroupState(-100999, "")
	if err != nil {
		t.Fatalf("GroupState olishda xato: %v", err)
	}
	if updatedState.CurrentLessonID != 2 || !updatedState.MorningSent {
		t.Errorf("GroupState to'g'ri saqlanmadi: %+v", updatedState)
	}

	// 6. Test Chat History
	for i := 1; i <= 10; i++ {
		_ = db.SaveChatMessage(-100999, "User", "Xabar", false)
	}

	history, err := db.GetRecentChatHistory(-100999, 8)
	if err != nil {
		t.Fatalf("Chat tarixini olishda xato: %v", err)
	}
	if len(history) != 8 {
		t.Errorf("Kutilgan tarix uzunligi: 8, lekin olindi: %d", len(history))
	}

	// 7. Test AI Logs & GetDayStats
	err = db.SaveAILog("generate_text", "gemini-2.5-flash", "Test prompt", "Test response", 120, "")
	if err != nil {
		t.Fatalf("SaveAILog xato: %v", err)
	}

	todayCount, err := db.GetTodayAICount()
	if err != nil || todayCount != 1 {
		t.Errorf("Kutilgan bugungi AI soni 1, lekin %d (err: %v)", todayCount, err)
	}

	logs, err := db.GetRecentAILogs(5)
	if err != nil || len(logs) != 1 {
		t.Fatalf("GetRecentAILogs kutilgan 1, lekin %d (err: %v)", len(logs), err)
	}
	if logs[0].ActionType != "generate_text" || logs[0].ModelName != "gemini-2.5-flash" {
		t.Errorf("AILog ma'lumotlari kutilgandek emas: %+v", logs[0])
	}

	today := time.Now().Format("2006-01-02")
	stats, err := db.GetDayStats(today)
	if err != nil {
		t.Fatalf("GetDayStats xato: %v", err)
	}
	if stats.SubmissionsCount != 1 || stats.TotalPointsToday != 15 {
		t.Errorf("DayStats kutilgandek emas: %+v", stats)
	}

	// 8. Test PausedDate and WeekendWishSent in GroupState
	state.PausedDate = today
	state.WeekendWishSent = true
	if err := db.UpdateGroupState(state); err != nil {
		t.Fatalf("UpdateGroupState pause xato: %v", err)
	}

	chkState, err := db.GetOrCreateGroupState(-100999, "")
	if err != nil || chkState.PausedDate != today || !chkState.WeekendWishSent {
		t.Errorf("PausedDate yoki WeekendWishSent to'g'ri saqlanmadi: %+v", chkState)
	}

	// 9. Test StudyContext
	studyCtx, err := db.GetStudyContext(-100999, 1, "Mavzu 1", "Chall 1", "Task 1", true, 12345, "Alisher")
	if err != nil {
		t.Fatalf("GetStudyContext xato: %v", err)
	}
	if studyCtx.TotalSubmissions != 1 || !studyCtx.UserHasSubmitted {
		t.Errorf("StudyContext kutilgandek emas: %+v", studyCtx)
	}
	if studyCtx.OfficialChallengeTitle != "Chall 1" || studyCtx.OfficialChallengeTask != "Task 1" {
		t.Errorf("StudyContext rasmiy topshiriq to'g'ri o'rnatilmadi: %+v", studyCtx)
	}
	formattedCtx := database.FormatStudyContext(studyCtx)
	if len(formattedCtx) == 0 {
		t.Errorf("Formatted study context bo'sh chiqdi")
	}

	// 10. Test GetMaxScoreForLesson
	maxScore, err := db.GetMaxScoreForLesson(12345, 1)
	if err != nil {
		t.Fatalf("GetMaxScoreForLesson xato: %v", err)
	}
	if maxScore != 15 {
		t.Errorf("GetMaxScoreForLesson kutilgan 15, lekin olindi %d", maxScore)
	}
}
