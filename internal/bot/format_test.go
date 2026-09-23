package bot_test

import (
	"strings"
	"testing"

	"learn-go-bot/internal/bot"
)

func TestMarkdownToTelegramHTML(t *testing.T) {
	input := "Voy-ey, Islombek, darsga bunchalik sho'ng'ib ketib, yuqoridagi xabarlarni o'qimay qoldingizmi yoki sezdirmay qochmoqchimisiz? 😉\n\n" +
		"Bugungi **#1: \"Go Asoslari: O'zgaruvchilar va Ma'lumot Turlari\"** mavzusi bo'yicha kechki kod topshirig'i guruhga biroz oldin tashlangan edi. Hozircha faqat @sanjarbek_engineer bajarib, +17 ballni cho'ntakka urib qo'ydi.\n\n" +
		"Marhamat, topshiriq sharti mana shundan iborat:\n" +
		"> **Topshiriq:** O'zingiz haqingizda ma'lumot saqlovchi o'zgaruvchilarni e'lon qiling (`ism`, `yosh`, `bo'yi`, `dasturchimi` kabi). Ularni qisqa e'lon qilish (`:=`) va an'anaviy (`var`) usulda yozib, `fmt.Printf` orqali chiroyli qilib konsolga chiqaring.\n\n" +
		"Qani, darsdagi faolligingizni amalda ham ko'rsating, tezroq kodingizni yozib guruhga tashlang-chi! Keyin siz aytgan o'sha qiyin savollarga o'tamiz! 🚀"

	htmlOutput := bot.MarkdownToTelegramHTML(input)

	// Check bold
	if !strings.Contains(htmlOutput, "<b>#1:") {
		t.Errorf("Bold topilmadi: %s", htmlOutput)
	}

	// Check mention with underscore preserved without broken tags
	if !strings.Contains(htmlOutput, "@sanjarbek_engineer") {
		t.Errorf("Username saqlanmadi: %s", htmlOutput)
	}

	// Check blockquote
	if !strings.Contains(htmlOutput, "<blockquote>") || !strings.Contains(htmlOutput, "</blockquote>") {
		t.Errorf("Blockquote topilmadi: %s", htmlOutput)
	}

	// Check code tags
	if !strings.Contains(htmlOutput, "<code>fmt.Printf</code>") {
		t.Errorf("Inline code topilmadi: %s", htmlOutput)
	}

	t.Logf("Formatted HTML:\n%s\n", htmlOutput)
}

func TestCodeBlocksInHTML(t *testing.T) {
	input := "Quyidagi kodni ko'ring:\n```go\nfunc main() {\n    fmt.Println(\"A < B & C > D\")\n}\n```\nO'rganing!"
	htmlOutput := bot.MarkdownToTelegramHTML(input)

	if !strings.Contains(htmlOutput, "<pre><code class=\"language-go\">") {
		t.Errorf("Code block topilmadi: %s", htmlOutput)
	}

	if !strings.Contains(htmlOutput, "&lt; B &amp; C &gt;") {
		t.Errorf("Code block ichidagi HTML belgilar escape qilinmadi: %s", htmlOutput)
	}
}
