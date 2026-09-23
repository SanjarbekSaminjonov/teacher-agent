package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

var ErrQuotaExceeded = errors.New("gemini_quota_exceeded: Barcha API kalitlarida token limiti tugadi")

func IsQuotaExceeded(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrQuotaExceeded) {
		return true
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "quota_exceeded") ||
		strings.Contains(errMsg, "resource_exhausted") ||
		strings.Contains(errMsg, "http 429") ||
		strings.Contains(errMsg, "quota exceeded")
}

type LogFunc func(actionType, model, prompt, response string, durationMs int64, errMsg string)

type DailyReportData struct {
	Date              string
	LessonTitle       string
	TotalChatMessages int
	BotChatMessages   int
	UserChatMessages  int
	SubmissionsCount  int
	TotalPointsToday  int
	TopLearnerName    string
	TopLearnerPoints  int
	MembersList       string
}

type Client struct {
	apiKeys       []string
	currentKeyIdx int
	keyMu         sync.Mutex
	models        []string
	httpClient    *http.Client
	logFunc       LogFunc
}

func (c *Client) SetLogFunc(fn LogFunc) {
	c.logFunc = fn
}

type geminiRequest struct {
	Contents         []geminiContent         `json:"contents"`
	GenerationConfig *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	Temperature     float64 `json:"temperature"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type ReviewResult struct {
	Feedback        string
	PointsEarned    int
	IsPassing       bool
	DetectedBlunder string
}

type AnswerResult struct {
	Answer          string
	DetectedBlunder string
}

func NewClient(apiKeys ...string) *Client {
	var validKeys []string
	for _, k := range apiKeys {
		k = strings.TrimSpace(k)
		if k != "" {
			validKeys = append(validKeys, k)
		}
	}

	return &Client{
		apiKeys: validKeys,
		models: []string{
			"gemini-3.1-flash-lite", // asosiy model (eng ko'p ishlatilgan)
			"gemini-3.5-flash-lite", // zaxira 1
			"gemini-3.8-flash",      // zaxira 2 (kuchliroq)
			"gemini-flash-lite-latest", // eski zaxira
		},
		httpClient: &http.Client{
			Timeout: 25 * time.Second,
		},
	}
}

func (c *Client) IsConfigured() bool {
	return len(c.apiKeys) > 0
}

func (c *Client) GenerateText(ctx context.Context, prompt string) (string, error) {
	if !c.IsConfigured() {
		return "", fmt.Errorf("GEMINI_API_KEY o'rnatilmagan")
	}

	startTime := time.Now()
	var lastErr error
	totalKeys := len(c.apiKeys)
	keysQuotaHit := 0

	for keyAttempt := 0; keyAttempt < totalKeys; keyAttempt++ {
		c.keyMu.Lock()
		currentKey := c.apiKeys[c.currentKeyIdx]
		c.keyMu.Unlock()

		quotaOnThisKey := false

		for _, model := range c.models {
			url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, currentKey)

			reqBody := geminiRequest{
				Contents: []geminiContent{
					{
						Role: "user",
						Parts: []geminiPart{
							{Text: prompt},
						},
					},
				},
				GenerationConfig: &geminiGenerationConfig{
					Temperature:     0.7,
					MaxOutputTokens: 600,
				},
			}

			jsonBytes, err := json.Marshal(reqBody)
			if err != nil {
				return "", err
			}

			req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
			if err != nil {
				lastErr = err
				continue
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := c.httpClient.Do(req)
			if err != nil {
				lastErr = fmt.Errorf("[%s] so'rovda xato: %w", model, err)
				continue
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				lastErr = err
				continue
			}

			bodyStr := string(body)
			if resp.StatusCode == http.StatusTooManyRequests ||
				strings.Contains(bodyStr, "RESOURCE_EXHAUSTED") ||
				strings.Contains(bodyStr, "Quota exceeded") {
				quotaOnThisKey = true
				lastErr = fmt.Errorf("[%s] HTTP %d: %w", model, resp.StatusCode, ErrQuotaExceeded)
				continue
			}

			if resp.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("[%s] HTTP %d: %s", model, resp.StatusCode, bodyStr)
				continue
			}

			var geminiResp geminiResponse
			if err := json.Unmarshal(body, &geminiResp); err != nil {
				lastErr = err
				continue
			}

			if geminiResp.Error != nil {
				if strings.Contains(geminiResp.Error.Message, "Quota exceeded") ||
					strings.Contains(geminiResp.Error.Message, "RESOURCE_EXHAUSTED") {
					quotaOnThisKey = true
					lastErr = fmt.Errorf("[%s] %w: %s", model, ErrQuotaExceeded, geminiResp.Error.Message)
					continue
				}
				lastErr = fmt.Errorf("[%s] API xatosi: %s", model, geminiResp.Error.Message)
				continue
			}

			if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
				lastErr = fmt.Errorf("[%s] bo'sh javob qaytardi", model)
				continue
			}

			resultText := strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text)
			if c.logFunc != nil {
				c.logFunc("generate_text", model, prompt, resultText, time.Since(startTime).Milliseconds(), "")
			}
			return resultText, nil
		}

		if quotaOnThisKey {
			keysQuotaHit++
			if totalKeys > 1 {
				c.keyMu.Lock()
				c.currentKeyIdx = (c.currentKeyIdx + 1) % totalKeys
				log.Printf("Gemini API kalitida kvota tugadi, navbatdagi kalitga o'tilmoqda (Index: %d)...\n", c.currentKeyIdx)
				c.keyMu.Unlock()
			}
		}
	}

	if c.logFunc != nil && lastErr != nil {
		c.logFunc("generate_text", "error", prompt, "", time.Since(startTime).Milliseconds(), lastErr.Error())
	}

	if keysQuotaHit >= totalKeys {
		return "", ErrQuotaExceeded
	}

	return "", fmt.Errorf("barcha zaxira Gemini kalitlari va modellari xatolik berdi. Oxirgi xato: %w", lastErr)
}

func (c *Client) GenerateIcebreaker(ctx context.Context, name, username, currentLessonTitle, previousNote string) (string, error) {
	if !c.IsConfigured() {
		targetName := name
		if username != "" {
			targetName = "@" + username
		}
		return fmt.Sprintf("👋 Salom %s! Go darsimizda faol bo'ling, savollaringiz bormi? 🐹", targetName), nil
	}

	target := name
	if username != "" {
		target = fmt.Sprintf("%s (@%s)", name, username)
	}

	var memoryContext string
	if strings.TrimSpace(previousNote) != "" {
		memoryContext = fmt.Sprintf("\nMentorning qayd daftarchasidagi eslatma (ushbu a'zo ilgari guruhda aytgan gap yoki qilgan xatosi): \"%s\". Ushbu gapni do'stona, samimiy va biroz gap bilan tekkizuvchi hazil bilan eslatib, bugungi darsga bog'lang!\n", previousNote)
	}

	prompt := fmt.Sprintf(`Siz Telegramdagi Go (Golang) o'rganish guruhining samimiy, hozirjavob, gap bilan tekkizishni yaxshi ko'radigan "Jonkuyar Senior Mentor"isiz.
Hozirda guruhda o'rganilayotgan Go mavzusi: "%s".
Guruh a'zosi: %s anchadan beri guruhda jim o'tiribdi, savol yoki kod yubormayapti.%s
Vazifangiz:
Ushbu a'zoga qarata o'zbekona, samimiy, hazilkash va gap bilan tekkizuvchi ohangda 2-3 jumlali chaqiriq yozing.
- Agar yuqorida daftarchadagi eslatma bo'lsa, albatta o'sha gapini eslatib, bugungi mavzuga bog'lang (masalan: "Bir paytlar '...' degandingiz, bugungi mavzu aynan shunga bag'ishlangan, qani topshiriqqa nima deysiz 😉").
- Agar eslatma bo'lmasa, burchakda jim kuzatib turganini payqaganingizni, Go o'rganishda tortinishga hojat yo'qligini do'stona hazil bilan ayting.
- Odamning ko'nglini og'ritmaydigan, yuziga tabassum yugurtirib suhbatga tortadigan bo'lsin.
- Agar username mavjud bo'lsa (@ bilan), albatta uni belgilab yozing.`, currentLessonTitle, target, memoryContext)

	return c.GenerateText(ctx, prompt)
}

func (c *Client) ReviewCode(ctx context.Context, lessonTitle, challengeTask, userCode, studentName string) (*ReviewResult, error) {
	if !c.IsConfigured() {
		// Heuristic fallback if no API key
		return &ReviewResult{
			Feedback:     "Ajoyib urinish! Kod topshirildi va hisobga olindi. 🚀\n(Eslatma: To'liq AI tahlili uchun GEMINI_API_KEY sozlang)",
			PointsEarned: 15,
			IsPassing:    true,
		}, nil
	}

	systemPrompt := fmt.Sprintf(`Siz Telegramdagi Go (Golang) o'rganish guruhining jiddiy, samimiy, tajribali va hurmatli o'qituvchi-mentorisiz.
Dars mavzusi: "%s"
Berilgan vazifa sharti:
"%s"

Kodni yuborgan o'quvchi: "%s"

O'quvchi yuborgan Go kodi:
`+"```go"+`
%s
`+"```"+`

Vazifangiz va Qat'iy Qoidalar:
1. MUOMALA VA HURMAT (QAT'IY):
- Kodni topshirgan o'quvchi: "%s". Koddagi o'zgaruvchilar ichida qanday ism bo'lishidan qat'i nazar, FAQAT kodingizni yuborgan "%s" ga hurmat bilan "Siz" deb murojaat qiling! Hech qachon "ukam", "og'ayni", "bratishka" kabi ko'cha iboralarini ishlatmang.
2. QAT'IY NO-SPOILERS QOIDASI:
- Agar o'quvchi kodida kamchilik yoki xato bo'lsa, HECH QACHON unga topshiriqning to'liq tayyor yechim kodini yozib bermang! Faqat qayerida xato borligini ko'rsating, tushuntiring, yo'nalish (hint) va maslahat bering. O'quvchi topshiriqni o'zi mustaqil yechishi shart!
3. Go standartlari (naming, formatting, idiomatic Go) bo'yicha qisqa, tushunarli mulohaza bering.
4. Javobingizni samimiy o'zbek tilida (Go terminlarini inglizcha qoldirib) bering.
5. DAFTARCHA QAYDI (Xotira): Agar o'quvchi kodida yoki izohida qiziq/kulgili xato, chalkashlik yoki Go tilidan shikoyat bo'lsa, javobingizning alohida qatoriga yozing:
DAFTAR: [1 jumla qisqa xulosa]
Agar bunday xarakterli xato bo'lmasa, DAFTAR qatorini umuman yozmang.
6. Javobingizning eng oxirgi qatorida QAT'IY ravishda mana shu formatda ball yozing:
BALL: [1 dan 20 gacha son]`, lessonTitle, challengeTask, studentName, userCode, studentName, studentName)

	respText, err := c.GenerateText(ctx, systemPrompt)
	if err != nil {
		return nil, err
	}

	points := 15
	isPassing := true
	var detectedBlunder string

	// Parse points and blunder from output
	lines := strings.Split(respText, "\n")
	var cleanedLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "BALL:") {
			var p int
			if _, err := fmt.Sscanf(trimmed, "BALL: %d", &p); err == nil {
				if p > 0 && p <= 20 {
					points = p
				}
			}
			continue
		}
		if strings.HasPrefix(trimmed, "DAFTAR:") {
			detectedBlunder = strings.TrimSpace(strings.TrimPrefix(trimmed, "DAFTAR:"))
			continue
		}
		cleanedLines = append(cleanedLines, line)
	}

	cleanFeedback := strings.TrimSpace(strings.Join(cleanedLines, "\n"))

	return &ReviewResult{
		Feedback:        cleanFeedback,
		PointsEarned:    points,
		IsPassing:       isPassing,
		DetectedBlunder: detectedBlunder,
	}, nil
}

func (c *Client) AnswerQuestion(ctx context.Context, question, currentLessonTitle, replyContext, chatHistory, previousNote, studyContext string) (*AnswerResult, error) {
	if !c.IsConfigured() {
		return &AnswerResult{
			Answer: "Savolingiz uchun rahmat! AI tahlili uchun GEMINI_API_KEY sozlanishi lozim.",
		}, nil
	}

	var contextBuilder strings.Builder
	contextBuilder.WriteString(fmt.Sprintf("Hozirda guruhda o'rganilayotgan Go darsi mavzusi: \"%s\".\n\n", currentLessonTitle))

	if strings.TrimSpace(studyContext) != "" {
		contextBuilder.WriteString("--- Bugungi dars va topshiriqning joriy holati ---\n")
		contextBuilder.WriteString(studyContext)
		contextBuilder.WriteString("--------------------------------------------------\n\n")
	}

	if strings.TrimSpace(previousNote) != "" {
		contextBuilder.WriteString(fmt.Sprintf("--- Mentorning xotirasi (ushbu o'quvchi ilgari aytgan gapi yoki xatosi) ---\n\"%s\"\n(Agar o'rinli bo'lsa, javobingizda ushbu gapni yengil hazil bilan eslatib o'ting)\n----------------------------------------------------\n\n", previousNote))
	}

	if strings.TrimSpace(chatHistory) != "" {
		contextBuilder.WriteString("--- Guruhdagi oxirgi muloqot tarixi (oxirgi 7-9 ta xabar) ---\n")
		contextBuilder.WriteString(chatHistory)
		contextBuilder.WriteString("\n----------------------------------------------------\n\n")
	}

	if strings.TrimSpace(replyContext) != "" {
		contextBuilder.WriteString("--- Foydalanuvchi quyidagi xabarga javob (reply) bermoqda ---\n\"\"\"\n")
		contextBuilder.WriteString(replyContext)
		contextBuilder.WriteString("\n\"\"\"\n----------------------------------------------------\n\n")
	}

	prompt := fmt.Sprintf(`Siz Telegramdagi Go (Golang) o'rganish guruhining jiddiy, samimiy, tajribali va hurmatli o'qituvchi-mentorisiz.
%sFoydalanuvchining joriy savoli / murojaati:
"%s"

Javob berish talablari va Qat'iy Qoidalar:
1. PEDAGOGIK ODOB VA MUOMALA MADANIYATI (QAT'IY):
- O'quvchilar bilan tengqur yoki ko'cha tilida gaplashmang! "Ukam", "bratishka", "zo'ri kim", "tish-tirnog'i bilan quvish", "gumburillatib" kabi ko'cha jargonlarini mutlaqo ISHLATMANG! Barcha o'quvchilarga hurmat bilan, do'stona tarzda "Siz" deb murojaat qiling.
- Darsdan tashqari bekorchi yoki provokatsion gaplar (masalan: 'podshohim de', 'kim zo'r', 'profil rasmlarni ko'rasanmi', 'uka qivoldingmi') bo'lsa, aslo tortishuvga KIRMANG! Bosiq va samimiy 1 ta jumla bilan javob berib, darhol darsga va Go dasturlashga qaytaring (masalan: "Kelishdik! Lekin keling, yaxshisi e'tiborimizni bugungi Go mavzusiga qaratsak. O'zgaruvchilar bo'yicha savolingiz bormi?").
2. VAQT VA KURS HOLATI:
- Bot bugun (1-kuni) ishga tushdi. Kecha hech qanday dars yoki topshiriq bo'lmagan. HECH QACHON "kecha", "kechagi topshiriqlar", "kecha aytganingizdek" deb gallyutsinatsiya qilmang!
3. REYTING VA BALLAR:
- Agar foydalanuvchi "menda necha ball bor?", "mening ballim qancha?", "kimda necha ball?", "reyting" deb so'rasa, faqat va faqat kontekstdagi rasmiy reyting jadvalidagi aniq raqamlarni ayting! O'zingizdan raqam to'qimang yoki ballarni qo'shib yubormang.
4. SPAM QILMASLIK VA BEZOR QILMASLIK:
- Har bir javobingiz oxirida topshirmagan o'quvchilarning username'larini ro'yxat qilib qayta-qayta chaqirmang. Topshiriqni eslatish faqat o'quvchi topshiriq yoki navbatdagi qadam haqida so'ragandagina o'rinli bo'ladi.
5. TOPSHIRIQ SHARTI (QAT'IY QOIDA):
- Agar foydalanuvchi "topshiriq nima?", "qani topshiriq?", "vazifa shartini ber", "haqiqiy topshiriqni tashla", "qaysi topshiriq?" kabi savol bersa, HECH QACHON o'zingizdan yangi yoki o'zgartirilgan topshiriq to'qimang! Faqat va faqat kontekstdagi "RASMIY AMALIY TOPSHIRIQ (CHALLENGE)" shartini to'liq va so'zma-so'z ko'rsating.
6. Agar savol kod bilan tushuntirishni talab qilsa, kichik va ixcham Go kodi namunasini keltiring. Lekin topshiriqning tayyor kodini emas, faqat mavzuga oid sintaksis misolini keltiring!
7. DAFTARCHA QAYDI (Xotira): Agar foydalanuvchi ushbu xabarida botga yoki Go tiliga nisbatan e'tiroz, tanqid, noto'g'ri/shubhali iddao yoki qiziq bahs bildirgan bo'lsa (masalan: "Go noqulay", "bot noto'g'ri aytyapti", "pointer keraksiz"), javobingizning eng oxirgi qatoriga alohida qatorda yozing:
DAFTAR: [qisqa 1 jumla xulosa]
Agar bunday e'tiroz/tanqid bo'lmasa, DAFTAR qatorini umuman yozmang.`, contextBuilder.String(), question)

	respText, err := c.GenerateText(ctx, prompt)
	if err != nil {
		return nil, err
	}

	var detectedBlunder string
	lines := strings.Split(respText, "\n")
	var cleanedLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "DAFTAR:") {
			detectedBlunder = strings.TrimSpace(strings.TrimPrefix(trimmed, "DAFTAR:"))
			continue
		}
		cleanedLines = append(cleanedLines, line)
	}

	cleanAnswer := strings.TrimSpace(strings.Join(cleanedLines, "\n"))

	return &AnswerResult{
		Answer:          cleanAnswer,
		DetectedBlunder: detectedBlunder,
	}, nil
}

func (c *Client) GenerateBatchSummary(ctx context.Context, events []string, currentLessonTitle, studyContext string) (string, error) {
	if !c.IsConfigured() {
		return "", nil
	}

	var eventsText strings.Builder
	for i, e := range events {
		eventsText.WriteString(fmt.Sprintf("%d. %s\n", i+1, e))
	}

	studyContextSection := ""
	if strings.TrimSpace(studyContext) != "" {
		studyContextSection = fmt.Sprintf("\n--- Guruhdagi bugungi amaliyot va topshiriq holati ---\n%s-----------------------------------------------------\n", studyContext)
	}

	prompt := fmt.Sprintf(`Siz Telegramdagi Go (Golang) o'rganish guruhining samimiy, hozirjavob, faol va xushchaqchaq "Senior Mentor"isiz.
Hozirda guruhda o'rganilayotgan Go mavzusi: "%s".
%s
Oxirgi daqiqalarda guruhda quyidagi gap-so'zlar yoki hodisalar bo'lib o'tdi va hozir guruh tinchidi:
%s
Vazifangiz:
Guruhda do'stona, qiziqarli va interaktiv muhit yaratish uchun ushbu suhbatga munosabat bildiring:
1. Agar suhbat bot haqida, rasm, dars, Go dasturlash yoki umumiy qiziq mavzular haqida bo'lsa, samimiy hazil, o'zbekona kinoya yoki rag'batlantirish bilan 1-2 jumlada quvnoq munosabat bildiring.
2. Gapni oxirida darsga yoki amaliyotga burib, hammani Go o'rganishga undab qo'ying.
3. TOPSHIRIQ KONTEKSTI: Agar bugungi topshiriq allaqachon e'lon qilingan bo'lsa va a'zolar darsdan chalg'ib gaplashib o'tirishgan bo'lsa yoki 'nima qilamiz?', 'zerikdik' deyishsa:
Kontekstdan foydalanib, kim topshirganini aytib, hali topshirmaganlarni topshiriqni yechishga undab qo'ying (Masalan: 'Eee do'stlar, bekor gaplashib o'tirmasdan bugungi topshiriqni yechinglar! Hozircha faqat [Falonchi] topshirdi, qolganlar qani?').
4. Agar biror a'zoning username'i berilgan bo'lsa (@ bilan), albatta uni belgilab yozing.
5. QAT'IY QOIDA: Agar a'zolarning gaplari mutlaqo shaxsiy bo'lsa va mentor sifatida aralashish noo'rin/noqulay bo'lsa, unda QAT'IY ravishda faqat bitta so'z yozing: SKIP`, currentLessonTitle, studyContextSection, eventsText.String())

	respText, err := c.GenerateText(ctx, prompt)
	if err != nil {
		return "", err
	}

	respTrimmed := strings.TrimSpace(respText)
	if strings.HasPrefix(strings.ToUpper(respTrimmed), "SKIP") || respTrimmed == "" {
		return "", nil
	}

	return respTrimmed, nil
}

func (c *Client) GenerateDailyReport(ctx context.Context, data *DailyReportData) (string, error) {
	if data == nil {
		return "Hisobot ma'lumotlari mavjud emas.", nil
	}
	if !c.IsConfigured() {
		return "Bugungi hisobot: Gemini AI sozlanmagan.", nil
	}

	members := data.MembersList
	if strings.TrimSpace(members) == "" {
		members = "Hozircha a'zolar passiv"
	}

	topLearnerInfo := "Hozircha yo'q"
	if data.TopLearnerName != "" {
		topLearnerInfo = fmt.Sprintf("%s (+%d ball)", data.TopLearnerName, data.TopLearnerPoints)
	}

	prompt := fmt.Sprintf(`Siz Telegramdagi Go (Golang) o'rganish guruhining bosh Ta'lim Maslahatchisi va Bosh Mentorisiz.
Siz guruh administratori (Sanjarbek) uchun bugungi o'quv kuni yuzasidan shaxsiy tahliliy hisobot (Executive Daily Report) tuzishingiz kerak.

Bugungi statistika (%s):
- O'rganilgan mavzu: "%s"
- Guruhdagi jami xabarlar: %d ta (A'zolar yozgani: %d ta, Bot yozgani: %d ta)
- Kod topshirganlar: %d ta
- Bugungi to'plangan ballar: %d ball
- Bugungi eng faol o'quvchi: %s
- Guruhdagi ro'yxatdan o'tgan a'zolar: %s

Vazifangiz:
Administratorga qaratilgan, professional, samimiy va chuqur pedagogik tahlil beruvchi hisobot yozing.
Quyidagi bandlarni albatta o'z ichiga olsin:
1. 📊 Kunlik Umumiy Holat: Bugungi dars dinamikasi va guruh faolligi qisqa sharhi.
2. 🏆 A'zolar Dinamikasi: Kimlar bugun o'zini ko'rsatdi, kimlar hali passiv/orqada qolmoqda va ularni qanday faollashtirish mumkin.
3. 🧠 O'rganish Sifati va Tahlil: O'quvchilar bugungi mavzuni qay darajada o'zlashtirishyapti? Yuborilgan kodlar va savollardan kelib chiqib, tushunchalar qanday o'zlashtirildi.
4. 💡 Metodik Tavsiya va Xulosa: Bot bugun qanchalik foydalanuvchilarga foydali bo'ldi? Ertangi navbatdagi darsda qaysi jihatlarga ko'proq urg'u berishimiz kerak?

Javobni o'zbek tilida, professional va chiroyli formatlangan Telegram xabari shaklida tuzing.`,
		data.Date, data.LessonTitle, data.TotalChatMessages, data.UserChatMessages, data.BotChatMessages,
		data.SubmissionsCount, data.TotalPointsToday, topLearnerInfo, members)

	return c.GenerateText(ctx, prompt)
}
