# 🐹 Teacher Agent — Avtonom Go O'qituvchi Agent

Telegram guruhida Go (Golang) dasturlash tilini tizimli va interaktiv tarzda o'rgatuvchi **avtonom agent-bot**. Har kuni mustaqil ravishda dars beradi, viktorina o'tkazadi, kod topshiriqlarini Gemini AI orqali tekshiradi va o'quvchilar faolligini rag'batlantiradi.

---

## 🌟 Imkoniyatlar

### Avtonom Kunlik Jadval (Dushanba–Juma)
| Vaqt | Tadbir |
|------|--------|
| 🌅 10:00 | Yangi mavzu — nazariy dars |
| 🥪 13:00 | Telegram Quiz (Poll) |
| 💻 15:00 | Amaliy kod topshirig'i (Challenge) |
| 👀 17:00 | Deadline eslatmasi |
| 🛑 17:30 | Qabul yakunlandi + kunlik reyting |
| 📊 18:00 | Admin uchun tahliliy hisobot |

### Gemini AI Integratsiyasi
- Foydalanuvchi kodi Gemini AI tomonidan tekshiriladi — xatolar, maslahat va ball beriladi
- O'quvchilar savollariga pedagogik qoidalar asosida javob beradi (spoiler bermaydi, "Siz" deb murojaat qiladi)
- Multi-API kalit pool — bir kalit tugasa, keyingisiga avtomatik o'tadi

### Ballar Tizimi
- Quiz to'g'ri javobi: **+10 ball**
- Kod topshirig'i: **+15...+20 ball** (AI bahosi asosida)
- Qayta topshirishda faqat **eng yaxshi natija** hisoblanadi (resubmission support)
- 17:30 dan keyin yuborilgan kodlar tekshiriladi, lekin ball berilmaydi

### O'quv Dasturi — 10 ta bosqich
| # | Mavzu |
|---|-------|
| 1 | Go Asoslari: O'zgaruvchilar va Ma'lumot Turlari |
| 2 | Boshqaruv Konstruksiyalari: `if`, `switch`, `for` |
| 3 | To'plamlar: Massivlar, Slicelar, Maplar |
| 4 | Funksiyalar, Ko'p qiymat qaytarish, `defer` |
| 5 | Ko'rsatkichlar (`*`, `&`) |
| 6 | Strukturalar va Metodlar |
| 7 | Interfeyslar va Polimorfizm |
| 8 | Xatolar bilan ishlash (`error`, `%w`, `errors.Is`) |
| 9 | Konkurentlik: Goroutinalar, Kanallar, `select` |
| 10 | Standart Kutubxona va REST API |

---

## 🚀 O'rnatish

### Talablar
- Go 1.22+
- Telegram Bot Token ([BotFather](https://t.me/BotFather))
- Google Gemini API kalit ([AI Studio](https://aistudio.google.com/))

### 1. Sozlash

```bash
cp .env.example .env
```

`.env` faylini to'ldiring:

```env
TELEGRAM_BOT_TOKEN=your_bot_token_here
ADMIN_TELEGRAM_ID=your_telegram_id_here   # @userinfobot orqali bilib oling
TELEGRAM_GROUP_ID=-1001234567890           # guruh ID (ixtiyoriy)

# Bir yoki bir nechta Gemini API kalit (vergul bilan):
GEMINI_API_KEYS=AIzaSyD_key1,AIzaSyD_key2

# Dars soatlari (ixtiyoriy):
MORNING_HOUR=10
AFTERNOON_HOUR=13
EVENING_HOUR=15
NUDGE_HOUR=17

DRY_RUN=false
```

### 2. Ishga tushirish

```bash
go build -o teacher-agent ./cmd/bot
./teacher-agent
```

### 3. Systemd service (Linux, fon rejimida)

```bash
# Misol service fayli ~/.config/systemd/user/learn-go-bot.service
systemctl --user enable --now learn-go-bot
systemctl --user status learn-go-bot
```

---

## 💬 Buyruqlar

### Guruh buyruqlari
| Buyruq | Vazifasi |
|--------|----------|
| `/start` | Bot bilan tanishuv |
| `/today` | Bugungi mavzuni ko'rish |
| `/quiz` | Bugungi viktorina |
| `/challenge` | Bugungi kod topshirig'i |
| `/ask <savol>` | Go bo'yicha AI ga savol berish |
| `/leaderboard` | TOP-10 reyting |
| `/progress` | Kurs bo'yicha progress |
| `/help` | Qo'llanma |

### Admin buyruqlari
| Buyruq | Vazifasi |
|--------|----------|
| `/report` | Kunlik hisobotni hozir ko'rish |
| `/ai_logs` | Oxirgi AI so'rovlari logi |
| `/pause_today` | Bugungi darsni to'xtatish |
| `/resume_today` | Darsni qayta tiklash |
| `/next` | Qo'lda keyingi darsga o'tish |

---

## 📁 Loyiha Strukturasi

```
learn-go-bot/
├── cmd/bot/
│   └── main.go                    # Kirish nuqtasi, graceful shutdown
├── internal/
│   ├── ai/
│   │   └── gemini.go              # Gemini API client, multi-key pool
│   ├── bot/
│   │   ├── bot.go                 # Handler-lar, scoring logikasi
│   │   ├── buffer.go              # Chat debouncer (AI batch summary)
│   │   └── format.go              # Markdown → Telegram HTML konverter
│   ├── config/
│   │   └── config.go              # .env yuklash
│   ├── curriculum/
│   │   ├── curriculum.go          # YAML darslarni yuklash
│   │   └── lessons/               # 10 ta dars (YAML formatida)
│   ├── database/
│   │   ├── db.go                  # SQLite operatsiyalari
│   │   └── models.go              # Struct modellar
│   └── scheduler/
│       └── agent_scheduler.go     # Cron jadval + missed dispatch recovery
├── .env.example
├── .gitignore
└── go.mod
```

---

## 🗄️ Ma'lumotlar Bazasi

SQLite (`bot.db`) — quyidagi jadvallar:

| Jadval | Maqsad |
|--------|--------|
| `users` | Foydalanuvchilar, ballar, statistika |
| `group_state` | Guruh holati, bugungi flags |
| `challenge_submissions` | Kod topshiriqlari tarixi |
| `quiz_attempts` | Quiz javoblari |
| `chat_history` | Oxirgi xabarlar (AI kontekst uchun) |
| `user_notes` | Mentor daftarchasi (AI xotirasi) |
| `ai_logs` | Barcha Gemini API so'rovlari |

---

## ⚙️ Texnik Tafsilotlar

- **Til:** Go 1.22+
- **Telegram:** [telebot.v3](https://gopkg.in/telebot.v3)
- **AI:** Google Gemini API (REST, multi-model fallback)
- **DB:** SQLite ([modernc.org/sqlite](https://modernc.org/sqlite) — CGO yo'q)
- **Cron:** [robfig/cron](https://github.com/robfig/cron) — `time.Local` zonasida
- **Timezone:** Scheduler server mahalliy vaqtida ishlaydi
