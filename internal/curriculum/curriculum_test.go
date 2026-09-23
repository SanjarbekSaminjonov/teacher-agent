package curriculum_test

import (
	"testing"

	"teacher-agent/internal/curriculum"
)

func TestCurriculumManager(t *testing.T) {
	mgr, err := curriculum.NewManager()
	if err != nil {
		t.Fatalf("Curriculum manager yaratishda xato: %v", err)
	}

	total := mgr.TotalLessons()
	if total != 10 {
		t.Fatalf("Kutilgan darslar soni: 10, lekin topildi: %d", total)
	}

	lessons := mgr.GetAllLessons()
	for i, l := range lessons {
		expectedID := i + 1
		if l.ID != expectedID {
			t.Errorf("Dars ID noto'g'ri: kutilgan %d, olingan %d", expectedID, l.ID)
		}

		if l.Title == "" {
			t.Errorf("%d-darsda sarlavha (title) bo'sh", l.ID)
		}

		if l.Theory == "" {
			t.Errorf("%d-darsda nazariya (theory) bo'sh", l.ID)
		}

		// Check Quiz
		if l.Quiz.Question == "" {
			t.Errorf("%d-darsda quiz savoli bo'sh", l.ID)
		}
		if len(l.Quiz.Options) < 2 {
			t.Errorf("%d-darsda quiz variantlari kamida 2 ta bo'lishi kerak, lekin %d ta", l.ID, len(l.Quiz.Options))
		}
		if l.Quiz.CorrectOptionID < 0 || l.Quiz.CorrectOptionID >= len(l.Quiz.Options) {
			t.Errorf("%d-darsda to'g'ri javob indeksi noto'g'ri: %d (variantlar: %d ta)", l.ID, l.Quiz.CorrectOptionID, len(l.Quiz.Options))
		}
		if l.Quiz.Explanation == "" {
			t.Errorf("%d-darsda quiz izohi (explanation) bo'sh", l.ID)
		}

		// Check Challenge
		if l.Challenge.Title == "" {
			t.Errorf("%d-darsda challenge sarlavhasi bo'sh", l.ID)
		}
		if l.Challenge.Task == "" {
			t.Errorf("%d-darsda challenge topshirig'i bo'sh", l.ID)
		}
	}
}
