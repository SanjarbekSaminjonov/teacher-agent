# 🐹 Learn Go Bot — Telegram O'quv Guruhi Uchun Avtonom Agent

Ushbu bot Go dasturlash tilini o'rganayotgan Telegram guruh a'zolarini faollashtirish, ularga Go tilini noldan boshlab tizimli o'rgatish va guruhda interaktiv o'quv muhitini yaratish uchun mo'ljallangan **avtonom o'qituvchi-agent**dir.

Bot server talab qilmaydi — uni o'zingizning mahalliy kompyuteringizda (Linux / macOS / Windows) osongina ishga tushirishingiz mumkin.

---

## 🌟 Asosiy Imkoniyatlar

1. **Tizimli Go O'quv Dasturi (10 ta bosqich):**
   - 01. Go Asoslari, O'zgaruvchilar va Ma'lumot turlari
   - 02. Boshqaruv konstruksiyalari: `if`, `switch` va yagona `for` tsikli
   - 03. To'plamlar: Massivlar, Slaytlar (`append`, `make`) va Lug'atlar (`map`)
   - 04. Funksiyalar, Ko'p qiymat qaytarish va `defer` (LIFO)
   - 05. Ko'rsatkichlar (`pointers`, `*`, `&`)
   - 06. Strukturalar va Metodlar (`struct`, Value vs Pointer receivers)
   - 07. Interfeyslar va Polimorfizm (`interface`, `any`, type assertion)
   - 08. Xatolar bilan ishlash (`error`, wrapping `%w`, `errors.Is`, `errors.As`)
   - 09. Konkurentlik: Goroutinalar, WaitGroup va Kanallar (`channels`, `select`)
   - 10. Standart Kutubxona va REST API (`net/http`, `encoding/json`)

2. **Avtonom Kunlik Reja (Kun davomida avtomatik taqsimot):**
   - 🌅 **09:00 (Ertalab):** Yangi mavzu bo'yicha qisqa va amaliy nazariy dars.
   - 🥪 **14:00 (Tushda):** Mavzuni mustahkamlash uchun rasmiy Telegram Quiz (Poll).
   - 🌙 **19:00 (Kechqurun):** 10-15 daqiqalik amaliy kod topshirig'i (Challenge).
   - 👀 **21:00 (Nudge / Eslatma):** Agar hech kim topshiriq yechimini yubormasa, bot samimiy savollar bilan guruhni bahsga tortadi.

3. **Google Gemini AI Integratsiyasi (Kod Tahlili va Mentorlik):**
   - O'quvchilar guruhda topshiriq kodi yoki savol yozganda, Gemini AI kodni tekshiradi, xatolarni ko'rsatadi, to'g'ri yozish bo'yicha maslahat beradi va ball taqdim etadi.

4. **Geymifikatsiya va Reyting (Leaderboard):**
   - Viktorinaga to'g'ri javob uchun: **+10 ball**.
   - Kod topshirig'i topshirgani uchun: **+15...+20 ball**.
   - Guruh a'zolari `/leaderboard` buyrug'i orqali eng faol ishtirokchilar TOP-10 ro'yxatini ko'rishlari mumkin.

---

## 🚀 O'rnatish va Ishga Tushirish

### 1. Talablar
- Go (1.23 yoki undan yuqori). Agar kompyuteringizda yo'q bo'lsa, `~/.local/go/bin/go` allaqachon tayyorlangan.

### 2. Sozlash (.env fayli)
Loyihaning ildiz papkasidagi `.env` faylini oching va quyidagi qiymatlarni kiriting:

```env
# 1. Telegram Bot Token (t.me/BotFather orqali olinadi):
TELEGRAM_BOT_TOKEN=123456789:ABCdefGHIjklMNOpqrSTUvwxYZ

# 2. Google Gemini API Kaliti (https://aistudio.google.com/ orqali bepul olinadi):
GEMINI_API_KEY=AIzaSyD...

# 3. Darslar o'tiladigan Telegram guruh ID raqami:
# (Agar kiritmasangiz, bot guruhga qo'shilganda o'zi aniqlab oladi)
TELEGRAM_GROUP_ID=-1001234567890

# Dars soatlari (ixtiyoriy o'zgartirish mumkin):
MORNING_HOUR=9
AFTERNOON_HOUR=14
EVENING_HOUR=19
NUDGE_HOUR=21

# Tokensiz lokal sinash uchun true, haqiqiy bot uchun false:
DRY_RUN=false
```

### 3. Botni Ishga Tushirish

Terminalda quyidagi buyruqni bajaring:

```bash
go run ./cmd/bot
```

Yoki binar fayl sifatida yig'ib olish (compile):

```bash
go build -o learn-go-bot ./cmd/bot
./learn-go-bot
```

---

## 🤖 Guruhda Ishlatiladigan Buyruqlar

- `/start` — Bot bilan tanishuv va xush kelibsiz xabari.
- `/today` — Bugungi o'tilayotgan mavzuni ko'rish.
- `/quiz` — Bugungi mavzu bo'yicha viktorinani ochish.
- `/challenge` — Bugungi amaliy kod topshirig'ini ko'rish.
- `/leaderboard` — Guruh a'zolarining reytingini (ballarini) ko'rish.
- `/progress` — Kursning umumiy o'zlashtirilish darajasi (progress bar).
- `/next` — Guruh tayyor bo'lganda navbatdagi darsga qo'lda o'tkazish.
- `/help` — Qo'llanma va yordam.

---

## 📁 Loyiha Strukturasi

```
learn-go-bot/
├── cmd/
│   └── bot/main.go            # Dasturning kirish nuqtasi
├── internal/
│   ├── config/config.go       # .env sozlamalarini boshqarish
│   ├── database/              # SQLite (foydalanuvchilar, ballar, holat)
│   ├── curriculum/            # Darslar boshqaruvi va YAML fayllari
│   │   └── lessons/           # 10 ta darsning nazariyasi, quiz va amaliyoti
│   ├── ai/gemini.go           # Google Gemini AI integratsiyasi
│   ├── bot/bot.go             # Telegram bot hodisalari va buyruqlari
│   └── scheduler/             # Avtonom cron jadval (Morning, Quiz, Challenge, Nudge)
├── .env                       # Sozlamalar fayli
├── .env.example               # Namuna sozlamalar
├── go.mod                     # Go modullari
└── README.md                  # Hujjat
```
