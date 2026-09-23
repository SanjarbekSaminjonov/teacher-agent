package curriculum

import (
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed lessons/*.yaml
var embeddedLessons embed.FS

type Quiz struct {
	Question        string   `yaml:"question"`
	Options         []string `yaml:"options"`
	CorrectOptionID int      `yaml:"correct_option_id"`
	Explanation     string   `yaml:"explanation"`
}

type Challenge struct {
	Title       string   `yaml:"title"`
	Task        string   `yaml:"task"`
	StarterCode string   `yaml:"starter_code"`
	Hints       []string `yaml:"hints"`
}

type Lesson struct {
	ID        int       `yaml:"id"`
	Slug      string    `yaml:"slug"`
	Title     string    `yaml:"title"`
	Subtitle  string    `yaml:"subtitle"`
	Theory    string    `yaml:"theory"`
	Quiz      Quiz      `yaml:"quiz"`
	Challenge Challenge `yaml:"challenge"`
}

type Manager struct {
	lessons map[int]*Lesson
	ordered []*Lesson
}

func NewManager() (*Manager, error) {
	m := &Manager{
		lessons: make(map[int]*Lesson),
	}

	entries, err := embeddedLessons.ReadDir("lessons")
	if err != nil {
		return nil, fmt.Errorf("embedded lessons o'qishda xato: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		data, err := embeddedLessons.ReadFile(filepath.Join("lessons", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("dars faylini o'qishda xato (%s): %w", entry.Name(), err)
		}

		var lesson Lesson
		if err := yaml.Unmarshal(data, &lesson); err != nil {
			return nil, fmt.Errorf("darsni parse qilishda xato (%s): %w", entry.Name(), err)
		}

		m.lessons[lesson.ID] = &lesson
		m.ordered = append(m.ordered, &lesson)
	}

	sort.Slice(m.ordered, func(i, j int) bool {
		return m.ordered[i].ID < m.ordered[j].ID
	})

	return m, nil
}

func (m *Manager) GetLesson(id int) (*Lesson, bool) {
	lesson, exists := m.lessons[id]
	return lesson, exists
}

func (m *Manager) TotalLessons() int {
	return len(m.ordered)
}

func (m *Manager) GetAllLessons() []*Lesson {
	return m.ordered
}
