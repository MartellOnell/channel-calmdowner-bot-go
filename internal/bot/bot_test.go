package bot

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"channel-calmdowner-bot/internal/storage"
	"channel-calmdowner-bot/internal/telegram"
)

type mockTG struct {
	chats        map[int64]*telegram.Chat
	members      map[string]*telegram.ChatMember
	admins       map[int64][]telegram.ChatMember
	sent         []mockSentMessage
	deleted      []mockDeleteMessage
	getChatErr   error
	getMemberErr error
	getAdminsErr error
	deleteMsgErr error
}

type mockSentMessage struct {
	ChatID  int64
	Text    string
	ReplyID int64
}

type mockDeleteMessage struct {
	ChatID    int64
	MessageID int64
}

func newMockTG() *mockTG {
	return &mockTG{
		chats:   make(map[int64]*telegram.Chat),
		members: make(map[string]*telegram.ChatMember),
		admins:  make(map[int64][]telegram.ChatMember),
	}
}

func (m *mockTG) GetUpdates(offset int64, timeout int) ([]telegram.Update, error) {
	return nil, nil
}

func (m *mockTG) SendMessage(chatID int64, text string, replyToMsgID int64) error {
	m.sent = append(m.sent, mockSentMessage{ChatID: chatID, Text: text, ReplyID: replyToMsgID})
	return nil
}

func (m *mockTG) GetChat(chatID int64) (*telegram.Chat, error) {
	if m.getChatErr != nil {
		return nil, m.getChatErr
	}
	chat, ok := m.chats[chatID]
	if !ok {
		return &telegram.Chat{ID: chatID, Type: "channel", Title: fmt.Sprintf("Channel %d", chatID)}, nil
	}
	return chat, nil
}

func (m *mockTG) GetChatMember(chatID, userID int64) (*telegram.ChatMember, error) {
	if m.getMemberErr != nil {
		return nil, m.getMemberErr
	}
	key := fmt.Sprintf("%d:%d", chatID, userID)
	member, ok := m.members[key]
	if !ok {
		return nil, fmt.Errorf("member not found")
	}
	return member, nil
}

func (m *mockTG) GetChatAdministrators(chatID int64) ([]telegram.ChatMember, error) {
	if m.getAdminsErr != nil {
		return nil, m.getAdminsErr
	}
	return m.admins[chatID], nil
}

func (m *mockTG) DeleteMessage(chatID, messageID int64) error {
	if m.deleteMsgErr != nil {
		return m.deleteMsgErr
	}
	m.deleted = append(m.deleted, mockDeleteMessage{ChatID: chatID, MessageID: messageID})
	return nil
}

func (m *mockTG) setChat(chatID int64, chat *telegram.Chat) {
	m.chats[chatID] = chat
}

func (m *mockTG) setMember(chatID, userID int64, member *telegram.ChatMember) {
	key := fmt.Sprintf("%d:%d", chatID, userID)
	m.members[key] = member
}

func (m *mockTG) setAdmins(chatID int64, admins []telegram.ChatMember) {
	m.admins[chatID] = admins
}

func (m *mockTG) lastSent() *mockSentMessage {
	if len(m.sent) == 0 {
		return nil
	}
	return &m.sent[len(m.sent)-1]
}

func (m *mockTG) assertSent(t *testing.T, idx int, expectedSub string) {
	t.Helper()
	if idx >= len(m.sent) {
		t.Fatalf("expected at least %d sent messages, got %d", idx+1, len(m.sent))
	}
	msg := m.sent[idx]
	if !strings.Contains(msg.Text, expectedSub) {
		t.Errorf("sent[%d].Text = %q, want containing %q", idx, msg.Text, expectedSub)
	}
}

func (m *mockTG) assertSentCount(t *testing.T, want int) {
	t.Helper()
	if len(m.sent) != want {
		t.Errorf("sent count = %d, want %d", len(m.sent), want)
	}
}

func (m *mockTG) assertDeleted(t *testing.T, chatID, msgID int64) {
	t.Helper()
	for _, d := range m.deleted {
		if d.ChatID == chatID && d.MessageID == msgID {
			return
		}
	}
	t.Errorf("message (chat=%d, msg=%d) was not deleted", chatID, msgID)
}

func (m *mockTG) assertDeletedCount(t *testing.T, want int) {
	t.Helper()
	if len(m.deleted) != want {
		t.Errorf("deleted count = %d, want %d", len(m.deleted), want)
	}
}

func newTestBot(t *testing.T) (*Bot, *mockTG, *storage.SQLite) {
	t.Helper()
	s, err := storage.New(":memory:")
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	m := newMockTG()

	m.setMember(-1001, 999, &telegram.ChatMember{
		User:              telegram.User{ID: 999, FirstName: "Bot"},
		Status:            "administrator",
		CanDeleteMessages: true,
	})

	bot := New(999, m, s)
	return bot, m, s
}

func setupUserWithChannel(t *testing.T, s *storage.SQLite, userID, channelID int64, dailyLimit int) {
	t.Helper()
	s.RegisterUser(userID)
	s.SetUserChannel(userID, channelID)
	if dailyLimit > 0 {
		s.SetUserDailyLimit(userID, dailyLimit)
	}
}

func assertContains(t *testing.T, text, substr string) {
	t.Helper()
	if !strings.Contains(text, substr) {
		t.Errorf("expected %q to contain %q", text, substr)
	}
}

func assertNotContains(t *testing.T, text, substr string) {
	t.Helper()
	if strings.Contains(text, substr) {
		t.Errorf("%q should NOT contain %q", text, substr)
	}
}

func TestCmdStart_NewUser(t *testing.T) {
	bot, _, s := newTestBot(t)

	response := bot.cmdStart(111)
	assertContains(t, response, "Добро пожаловать!")
	assertContains(t, response, "не задан")
	assertContains(t, response, "Дневной лимит: 2")
	assertContains(t, response, "Активен")

	us, _ := s.GetUserSettings(111)
	if us == nil {
		t.Fatal("user should be registered")
	}
}

func TestCmdStart_ExistingUser(t *testing.T) {
	bot, _, s := newTestBot(t)
	s.RegisterUser(111)
	s.SetUserChannel(111, -1001234567890)
	s.SetUserDailyLimit(111, 5)

	response := bot.cmdStart(111)
	assertContains(t, response, "Добро пожаловать!")
	assertContains(t, response, "-1001234567890")
	assertContains(t, response, "Дневной лимит: 5")
}

func TestCmdHelp(t *testing.T) {
	bot, _, _ := newTestBot(t)
	response := bot.cmdHelp()
	assertContains(t, response, "Доступные команды:")
	assertContains(t, response, "/start")
	assertContains(t, response, "/addchannel")
	assertContains(t, response, "/setlimit")
}

func TestCmdMyChannel_NotRegistered(t *testing.T) {
	bot, _, _ := newTestBot(t)
	response := bot.cmdMyChannel(111)
	assertContains(t, response, "не зарегистрированы")
}

func TestCmdMyChannel_NoChannel(t *testing.T) {
	bot, _, s := newTestBot(t)
	s.RegisterUser(111)
	response := bot.cmdMyChannel(111)
	assertContains(t, response, "ID канала: не задан")
}

func TestCmdMyChannel_WithChannel(t *testing.T) {
	bot, _, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	response := bot.cmdMyChannel(111)
	assertContains(t, response, "ID канала: -1005")
	assertContains(t, response, "Дневной лимит: 2")
}

func TestCmdSetLimit_NotRegistered(t *testing.T) {
	bot, _, _ := newTestBot(t)
	response := bot.cmdSetLimit(111, []string{"5"})
	assertContains(t, response, "не зарегистрированы")
}

func TestCmdSetLimit_NoChannel(t *testing.T) {
	bot, _, s := newTestBot(t)
	s.RegisterUser(111)
	response := bot.cmdSetLimit(111, []string{"5"})
	assertContains(t, response, "Канал не привязан")
}

func TestCmdSetLimit_NoArgs(t *testing.T) {
	bot, _, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	response := bot.cmdSetLimit(111, []string{})
	assertContains(t, response, "Использование: /setlimit")
}

func TestCmdSetLimit_InvalidNumber(t *testing.T) {
	bot, _, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	response := bot.cmdSetLimit(111, []string{"abc"})
	assertContains(t, response, "Неверный лимит")
}

func TestCmdSetLimit_Zero(t *testing.T) {
	bot, _, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	response := bot.cmdSetLimit(111, []string{"0"})
	assertContains(t, response, "Неверный лимит")
}

func TestCmdSetLimit_Success(t *testing.T) {
	bot, _, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	response := bot.cmdSetLimit(111, []string{"10"})
	assertContains(t, response, "Дневной лимит установлен: 10")

	us, _ := s.GetUserSettings(111)
	if us.DailyLimit != 10 {
		t.Errorf("DailyLimit = %d, want 10", us.DailyLimit)
	}
}

func TestCmdAddChannel_NotRegistered(t *testing.T) {
	bot, _, _ := newTestBot(t)
	response := bot.cmdAddChannel(111, []string{"-1001234567890"})
	assertContains(t, response, "не зарегистрированы")
}

func TestCmdAddChannel_NoArgs(t *testing.T) {
	bot, _, s := newTestBot(t)
	s.RegisterUser(111)
	response := bot.cmdAddChannel(111, []string{})
	assertContains(t, response, "Использование: /addchannel")
}

func TestCmdAddChannel_InvalidID(t *testing.T) {
	bot, _, s := newTestBot(t)
	s.RegisterUser(111)
	response := bot.cmdAddChannel(111, []string{"notanumber"})
	assertContains(t, response, "Неверный ID канала")
}

func TestCmdAddChannel_ChatNotFound(t *testing.T) {
	bot, m, s := newTestBot(t)
	s.RegisterUser(111)
	m.getChatErr = fmt.Errorf("chat not found")
	response := bot.cmdAddChannel(111, []string{"-1001234567890"})
	assertContains(t, response, "не может получить доступ")
}

func TestCmdAddChannel_BotNotAdmin(t *testing.T) {
	bot, m, s := newTestBot(t)
	s.RegisterUser(111)
	m.setChat(-1001234567890, &telegram.Chat{ID: -1001234567890, Title: "My Channel", Type: "channel"})
	m.setMember(-1001234567890, 999, &telegram.ChatMember{
		User:   telegram.User{ID: 999, FirstName: "Bot"},
		Status: "member",
	})
	response := bot.cmdAddChannel(111, []string{"-1001234567890"})
	assertContains(t, response, "должен быть администратором")
}

func TestCmdAddChannel_BotAdminNoDelete(t *testing.T) {
	bot, m, s := newTestBot(t)
	s.RegisterUser(111)
	m.setChat(-1001234567890, &telegram.Chat{ID: -1001234567890, Title: "My Channel", Type: "channel"})
	m.setMember(-1001234567890, 999, &telegram.ChatMember{
		User:              telegram.User{ID: 999, FirstName: "Bot"},
		Status:            "administrator",
		CanDeleteMessages: false,
	})
	response := bot.cmdAddChannel(111, []string{"-1001234567890"})
	assertContains(t, response, "должен иметь право 'Удаление сообщений'")
}

func TestCmdAddChannel_Success(t *testing.T) {
	bot, m, s := newTestBot(t)
	s.RegisterUser(111)
	m.setChat(-1001234567890, &telegram.Chat{ID: -1001234567890, Title: "My Channel", Type: "channel"})
	m.setMember(-1001234567890, 999, &telegram.ChatMember{
		User:              telegram.User{ID: 999, FirstName: "Bot"},
		Status:            "administrator",
		CanDeleteMessages: true,
	})
	response := bot.cmdAddChannel(111, []string{"-1001234567890"})
	assertContains(t, response, "Канал привязан: My Channel")
	assertContains(t, response, "Бот будет отслеживать сообщения")

	us, _ := s.GetUserSettings(111)
	if !us.ChannelID.Valid || us.ChannelID.Int64 != -1001234567890 {
		t.Errorf("ChannelID = %v, want -1001234567890", us.ChannelID)
	}
}

func TestCmdAddChannel_BotIsCreator(t *testing.T) {
	bot, m, s := newTestBot(t)
	s.RegisterUser(111)
	m.setChat(-100777, &telegram.Chat{ID: -100777, Title: "Owner Channel", Type: "channel"})
	m.setMember(-100777, 999, &telegram.ChatMember{
		User:   telegram.User{ID: 999, FirstName: "Bot"},
		Status: "creator",
	})
	response := bot.cmdAddChannel(111, []string{"-100777"})
	assertContains(t, response, "Канал привязан")
}

func TestCmdAddChannel_ChatWithoutTitle(t *testing.T) {
	bot, m, s := newTestBot(t)
	s.RegisterUser(111)
	m.setChat(-100999, &telegram.Chat{ID: -100999, Type: "channel"})
	m.setMember(-100999, 999, &telegram.ChatMember{
		User:              telegram.User{ID: 999, FirstName: "Bot"},
		Status:            "administrator",
		CanDeleteMessages: true,
	})
	response := bot.cmdAddChannel(111, []string{"-100999"})
	assertContains(t, response, "-100999")
}

func TestCmdStatus_NotRegistered(t *testing.T) {
	bot, _, _ := newTestBot(t)
	response := bot.cmdStatus(111)
	assertContains(t, response, "не зарегистрированы")
}

func TestCmdStatus_NoActiveRestrictions(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	m.setChat(-1005, &telegram.Chat{ID: -1005, Title: "My Channel", Type: "channel"})
	response := bot.cmdStatus(111)
	assertContains(t, response, "Нет активных ограничений")
}

func TestCmdStatus_WithRestrictions(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	m.setChat(-1005, &telegram.Chat{ID: -1005, Title: "My Channel", Type: "channel"})

	until := time.Now().Add(24 * time.Hour)
	s.CreateRestriction(222, -1005, 1, until, "{}")
	s.CreateRestriction(333, -1005, 2, until.Add(48*time.Hour), "{}")

	response := bot.cmdStatus(111)
	assertContains(t, response, "Активные ограничения в My Channel")
	assertContains(t, response, "Админ ID: 222 — Нарушение #1")
	assertContains(t, response, "Админ ID: 333 — Нарушение #2")
}

func TestCmdUnrestrict_NotRegistered(t *testing.T) {
	bot, _, _ := newTestBot(t)
	response := bot.cmdUnrestrict(111, []string{"222"})
	assertContains(t, response, "не зарегистрированы")
}

func TestCmdUnrestrict_NoArgs(t *testing.T) {
	bot, _, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	response := bot.cmdUnrestrict(111, []string{})
	assertContains(t, response, "Использование: /unrestrict")
}

func TestCmdUnrestrict_Success(t *testing.T) {
	bot, _, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	until := time.Now().Add(24 * time.Hour)
	s.CreateRestriction(222, -1005, 1, until, "{}")
	response := bot.cmdUnrestrict(111, []string{"222"})
	assertContains(t, response, "Ограничение снято для пользователя 222")
	r, _ := s.ActiveRestriction(222, -1005)
	if r != nil {
		t.Error("restriction should be inactive")
	}
}

func TestCmdUnrestrictAll_Success(t *testing.T) {
	bot, _, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	until := time.Now().Add(24 * time.Hour)
	s.CreateRestriction(222, -1005, 1, until, "{}")
	s.CreateRestriction(333, -1005, 1, until, "{}")
	response := bot.cmdUnrestrictAll(111)
	assertContains(t, response, "Все ограничения сняты для этого канала")
	r1, _ := s.ActiveRestriction(222, -1005)
	r2, _ := s.ActiveRestriction(333, -1005)
	if r1 != nil || r2 != nil {
		t.Error("all restrictions should be inactive")
	}
}

func TestCmdExempt_Success(t *testing.T) {
	bot, _, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	response := bot.cmdExempt(111, []string{"222"})
	assertContains(t, response, "Пользователь 222 теперь исключён")
	exempt, _ := s.IsExempt(222, -1005)
	if !exempt {
		t.Error("user should be exempt")
	}
}

func TestCmdExemptRemove_Success(t *testing.T) {
	bot, _, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	s.AddExemption(222, -1005)
	response := bot.cmdExemptRemove(111, []string{"222"})
	assertContains(t, response, "Исключение убрано для пользователя 222")
	exempt, _ := s.IsExempt(222, -1005)
	if exempt {
		t.Error("user should not be exempt")
	}
}

func TestCmdExemptList_Empty(t *testing.T) {
	bot, _, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	response := bot.cmdExemptList(111)
	assertContains(t, response, "Нет исключений")
}

func TestCmdExemptList_WithExemptions(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	s.AddExemption(222, -1005)
	s.AddExemption(333, -1005)
	m.setMember(-1005, 222, &telegram.ChatMember{
		User:   telegram.User{ID: 222, FirstName: "Alice", Username: "alice"},
		Status: "administrator",
	})
	m.setMember(-1005, 333, &telegram.ChatMember{
		User:   telegram.User{ID: 333, FirstName: "Bob"},
		Status: "administrator",
	})
	response := bot.cmdExemptList(111)
	assertContains(t, response, "Исключённые администраторы:")
	assertContains(t, response, "Alice")
	assertContains(t, response, "@alice")
	assertContains(t, response, "Bob")
}

func TestCmdCheckAdmin_IsCreator(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	m.setMember(-1005, 999, &telegram.ChatMember{
		User:   telegram.User{ID: 999, FirstName: "Bot"},
		Status: "creator",
	})
	response := bot.cmdCheckAdmin(111)
	assertContains(t, response, "владелец канала")
}

func TestCmdCheckAdmin_NotAdmin(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	m.setMember(-1005, 999, &telegram.ChatMember{
		User:   telegram.User{ID: 999, FirstName: "Bot"},
		Status: "member",
	})
	response := bot.cmdCheckAdmin(111)
	assertContains(t, response, "Бот не является администратором")
}

func TestCmdCheckAdmin_AdminNoDelete(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	m.setMember(-1005, 999, &telegram.ChatMember{
		User:              telegram.User{ID: 999, FirstName: "Bot"},
		Status:            "administrator",
		CanDeleteMessages: false,
	})
	response := bot.cmdCheckAdmin(111)
	assertContains(t, response, "ПРЕДУПРЕЖДЕНИЕ")
}

func TestCmdAdmins_Success(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	m.setChat(-1005, &telegram.Chat{ID: -1005, Title: "Test Channel", Type: "channel"})
	m.setAdmins(-1005, []telegram.ChatMember{
		{User: telegram.User{ID: 10, FirstName: "Owner", Username: "owner"}, Status: "creator"},
		{User: telegram.User{ID: 20, FirstName: "Alice", LastName: "Smith", Username: "alice"}, Status: "administrator"},
		{User: telegram.User{ID: 30, FirstName: "Bob"}, Status: "administrator"},
		{User: telegram.User{ID: 40, FirstName: "Regular"}, Status: "member"},
	})
	response := bot.cmdAdmins(111)
	assertContains(t, response, "Администраторы в Test Channel")
	assertContains(t, response, "Владелец")
	assertContains(t, response, "@owner")
	assertContains(t, response, "Alice Smith")
	assertContains(t, response, "@alice")
	assertContains(t, response, "Bob")
	assertContains(t, response, "Активен")
	assertNotContains(t, response, "Regular")
	if !strings.Contains(response, "3. ") || strings.Contains(response, "4. ") {
		t.Error("expected exactly 3 admin entries")
	}
}

func TestCmdAdmins_WithRestricted(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	m.setChat(-1005, &telegram.Chat{ID: -1005, Title: "Test Channel", Type: "channel"})
	m.setAdmins(-1005, []telegram.ChatMember{
		{User: telegram.User{ID: 10, FirstName: "Owner"}, Status: "creator"},
		{User: telegram.User{ID: 20, FirstName: "Alice"}, Status: "administrator"},
	})
	until := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	s.CreateRestriction(20, -1005, 1, until, "{}")
	response := bot.cmdAdmins(111)
	assertContains(t, response, "Ограничен (до 2026-07-01 12:00)")
}

func TestCmdAdmins_WithExempt(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	m.setChat(-1005, &telegram.Chat{ID: -1005, Title: "Test Channel", Type: "channel"})
	m.setAdmins(-1005, []telegram.ChatMember{
		{User: telegram.User{ID: 10, FirstName: "Owner"}, Status: "creator"},
		{User: telegram.User{ID: 20, FirstName: "Alice"}, Status: "administrator"},
	})
	s.AddExemption(20, -1005)
	response := bot.cmdAdmins(111)
	assertContains(t, response, "Исключён")
}

func TestRestrictionDuration(t *testing.T) {
	tests := []struct {
		offense  int
		wantDays int64
	}{
		{1, 1}, {2, 2}, {3, 4}, {4, 8}, {5, 16},
	}
	for _, tt := range tests {
		d := restrictionDuration(tt.offense)
		gotDays := int64(d.Hours()) / 24
		if gotDays != tt.wantDays {
			t.Errorf("restrictionDuration(%d) = %d days, want %d", tt.offense, gotDays, tt.wantDays)
		}
	}
}

func TestRestrictionDuration_ZeroOrNegative(t *testing.T) {
	d := restrictionDuration(0)
	if d != 24*time.Hour {
		t.Errorf("restrictionDuration(0) = %v, want 24h", d)
	}
	d = restrictionDuration(-1)
	if d != 24*time.Hour {
		t.Errorf("restrictionDuration(-1) = %v, want 24h", d)
	}
}

func TestRestrictionDuration_Capped(t *testing.T) {
	d := restrictionDuration(100)
	if d <= 0 {
		t.Error("restrictionDuration(100) should return a positive duration, not overflow")
	}
	if d < 100*365*24*time.Hour {
		t.Error("restrictionDuration(100) should be a very large (effectively forever) duration")
	}
}

func TestParseInt64(t *testing.T) {
	v, _ := parseInt64("123456")
	if v != 123456 {
		t.Errorf("= %d, want 123456", v)
	}
	v, _ = parseInt64("-1001234567890")
	if v != -1001234567890 {
		t.Errorf("= %d", v)
	}
	_, err := parseInt64("not_a_number")
	if err == nil {
		t.Error("expected error")
	}
}

func TestRequireSettings(t *testing.T) {
	bot, _, s := newTestBot(t)
	_, errMsg := bot.requireSettings(111)
	assertContains(t, errMsg, "не зарегистрированы")

	s.RegisterUser(111)
	_, errMsg = bot.requireSettings(111)
	assertContains(t, errMsg, "Канал не привязан")

	setupUserWithChannel(t, s, 111, -1005, 2)
	us, errMsg := bot.requireSettings(111)
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if us.ChannelID.Int64 != -1005 {
		t.Errorf("ChannelID = %d", us.ChannelID.Int64)
	}
}

func TestHandleUpdate_ChannelPost(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	m.setMember(-1005, 999, &telegram.ChatMember{
		User:              telegram.User{ID: 999, FirstName: "Bot"},
		Status:            "administrator",
		CanDeleteMessages: true,
	})
	update := telegram.Update{
		UpdateID:    1,
		ChannelPost: &telegram.Message{MessageID: 10, From: &telegram.User{ID: 222, FirstName: "Admin"}, Chat: telegram.Chat{ID: -1005, Type: "channel"}},
	}
	bot.handleUpdate(update)
	count, _ := s.CountToday(222, -1005)
	if count != 1 {
		t.Errorf("message count = %d, want 1", count)
	}
}

func TestHandleUpdate_ChannelPost_OverLimit(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 1)
	m.setMember(-1005, 999, &telegram.ChatMember{
		User:              telegram.User{ID: 999, FirstName: "Bot"},
		Status:            "administrator",
		CanDeleteMessages: true,
	})
	s.LogMessage(222, -1005)
	update := telegram.Update{
		UpdateID:    1,
		ChannelPost: &telegram.Message{MessageID: 10, From: &telegram.User{ID: 222, FirstName: "Admin"}, Chat: telegram.Chat{ID: -1005, Type: "channel"}},
	}
	bot.handleUpdate(update)
	m.assertDeleted(t, -1005, 10)
	r, _ := s.ActiveRestriction(222, -1005)
	if r == nil {
		t.Fatal("restriction should have been created")
	}
	if r.Offense != 1 {
		t.Errorf("Offense = %d, want 1", r.Offense)
	}
}

func TestHandleUpdate_ChannelPost_Exempt(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 1)
	m.setMember(-1005, 999, &telegram.ChatMember{
		User:              telegram.User{ID: 999, FirstName: "Bot"},
		Status:            "administrator",
		CanDeleteMessages: true,
	})
	s.AddExemption(222, -1005)
	update := telegram.Update{
		UpdateID:    1,
		ChannelPost: &telegram.Message{MessageID: 10, From: &telegram.User{ID: 222, FirstName: "Admin"}, Chat: telegram.Chat{ID: -1005, Type: "channel"}},
	}
	bot.handleUpdate(update)
	m.assertDeletedCount(t, 0)
	count, _ := s.CountToday(222, -1005)
	if count != 0 {
		t.Errorf("message count = %d, want 0 (exempt messages NOT logged)", count)
	}
}

func TestHandleUpdate_ChannelPost_AlreadyRestricted(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	m.setMember(-1005, 999, &telegram.ChatMember{
		User:              telegram.User{ID: 999, FirstName: "Bot"},
		Status:            "administrator",
		CanDeleteMessages: true,
	})
	until := time.Now().Add(24 * time.Hour)
	s.CreateRestriction(222, -1005, 1, until, "{}")
	update := telegram.Update{
		UpdateID:    1,
		ChannelPost: &telegram.Message{MessageID: 10, From: &telegram.User{ID: 222, FirstName: "Admin"}, Chat: telegram.Chat{ID: -1005, Type: "channel"}},
	}
	bot.handleUpdate(update)
	m.assertDeleted(t, -1005, 10)
	count, _ := s.CountToday(222, -1005)
	if count != 0 {
		t.Errorf("restricted user's messages should not be logged, got %d", count)
	}
}

func TestHandleUpdate_ChannelPost_Paused(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	s.SetUserActive(111, false)
	m.setMember(-1005, 999, &telegram.ChatMember{
		User:              telegram.User{ID: 999, FirstName: "Bot"},
		Status:            "administrator",
		CanDeleteMessages: true,
	})
	update := telegram.Update{
		UpdateID:    1,
		ChannelPost: &telegram.Message{MessageID: 10, From: &telegram.User{ID: 222, FirstName: "Admin"}, Chat: telegram.Chat{ID: -1005, Type: "channel"}},
	}
	bot.handleUpdate(update)
	count, _ := s.CountToday(222, -1005)
	if count != 0 {
		t.Errorf("message count = %d, want 0 (paused)", count)
	}
}

func TestHandleUpdate_ChannelPost_CantRestrict(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	m.setMember(-1005, 999, &telegram.ChatMember{
		User:   telegram.User{ID: 999, FirstName: "Bot"},
		Status: "member",
	})
	update := telegram.Update{
		UpdateID:    1,
		ChannelPost: &telegram.Message{MessageID: 10, From: &telegram.User{ID: 222, FirstName: "Admin"}, Chat: telegram.Chat{ID: -1005, Type: "channel"}},
	}
	bot.handleUpdate(update)
	count, _ := s.CountToday(222, -1005)
	if count != 0 {
		t.Errorf("message count = %d, want 0 (bot can't restrict)", count)
	}
}

func TestHandleMessage_Command(t *testing.T) {
	bot, m, _ := newTestBot(t)
	msg := &telegram.Message{
		MessageID: 5,
		From:      &telegram.User{ID: 111, FirstName: "User"},
		Chat:      telegram.Chat{ID: 111, Type: "private"},
		Text:      "/help",
	}
	bot.handleMessage(msg)
	m.assertSent(t, 0, "Доступные команды:")
}

func TestHandleMessage_NotCommand(t *testing.T) {
	bot, m, _ := newTestBot(t)
	msg := &telegram.Message{
		MessageID: 5,
		From:      &telegram.User{ID: 111, FirstName: "User"},
		Chat:      telegram.Chat{ID: 111, Type: "private"},
		Text:      "Hello bot",
	}
	bot.handleMessage(msg)
	m.assertSentCount(t, 0)
}

func TestHandleMessage_UnknownCommand(t *testing.T) {
	bot, m, _ := newTestBot(t)
	msg := &telegram.Message{
		MessageID: 5,
		From:      &telegram.User{ID: 111, FirstName: "User"},
		Chat:      telegram.Chat{ID: 111, Type: "private"},
		Text:      "/unknown_cmd",
	}
	bot.handleMessage(msg)
	m.assertSentCount(t, 0)
}

func TestHandleMessage_StripBotName(t *testing.T) {
	bot, m, _ := newTestBot(t)
	msg := &telegram.Message{
		MessageID: 5,
		From:      &telegram.User{ID: 111, FirstName: "User"},
		Chat:      telegram.Chat{ID: 111, Type: "private"},
		Text:      "/help@my_bot",
	}
	bot.handleMessage(msg)
	m.assertSentCount(t, 1)
	m.assertSent(t, 0, "Доступные команды:")
}

func TestHandleMessage_ReplyTo(t *testing.T) {
	bot, m, _ := newTestBot(t)
	msg := &telegram.Message{
		MessageID: 5,
		From:      &telegram.User{ID: 111, FirstName: "User"},
		Chat:      telegram.Chat{ID: 111, Type: "private"},
		Text:      "/start",
	}
	bot.handleMessage(msg)
	if len(m.sent) != 1 {
		t.Fatal("expected 1 sent message")
	}
	if m.sent[0].ReplyID != 5 {
		t.Errorf("ReplyID = %d, want 5", m.sent[0].ReplyID)
	}
}

func TestApplyRestriction_Permissions(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	m.setMember(-1005, 222, &telegram.ChatMember{
		User:              telegram.User{ID: 222, FirstName: "Admin"},
		Status:            "administrator",
		CanDeleteMessages: true,
		CanPostMessages:   false,
	})
	bot.applyRestriction(222, -1005)
	r, _ := s.ActiveRestriction(222, -1005)
	if r == nil {
		t.Fatal("restriction should exist")
	}
	var perms map[string]bool
	json.Unmarshal([]byte(r.Permissions), &perms)
	if !perms["can_delete_messages"] {
		t.Error("can_delete_messages should be true")
	}
	if perms["can_post_messages"] {
		t.Error("can_post_messages should be false")
	}
}

func TestApplyRestriction_Escalation(t *testing.T) {
	bot, m, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	m.setMember(-1005, 222, &telegram.ChatMember{
		User:   telegram.User{ID: 222, FirstName: "Admin"},
		Status: "administrator",
	})

	bot.applyRestriction(222, -1005)
	r1, _ := s.ActiveRestriction(222, -1005)
	if r1.Offense != 1 {
		t.Fatalf("first offense = %d, want 1", r1.Offense)
	}
	d1 := int64(r1.Until.Sub(r1.Created).Hours()) / 24
	if d1 != 1 {
		t.Errorf("first duration = %d days, want 1", d1)
	}

	s.LiftRestriction(222, -1005)
	bot.applyRestriction(222, -1005)
	r2, _ := s.ActiveRestriction(222, -1005)
	if r2.Offense != 2 {
		t.Fatalf("second offense = %d, want 2", r2.Offense)
	}
	d2 := int64(r2.Until.Sub(r2.Created).Hours()) / 24
	if d2 != 2 {
		t.Errorf("second duration = %d days, want 2", d2)
	}

	s.LiftRestriction(222, -1005)
	bot.applyRestriction(222, -1005)
	r3, _ := s.ActiveRestriction(222, -1005)
	if r3.Offense != 3 {
		t.Fatalf("third offense = %d, want 3", r3.Offense)
	}
	d3 := int64(r3.Until.Sub(r3.Created).Hours()) / 24
	if d3 != 4 {
		t.Errorf("third duration = %d days, want 4", d3)
	}
}

func TestCheckExpired_LiftsExpired(t *testing.T) {
	bot, _, s := newTestBot(t)
	pastUntil := time.Now().Add(-24 * time.Hour)
	futureUntil := time.Now().Add(24 * time.Hour)
	s.CreateRestriction(111, -1005, 1, pastUntil, "{}")
	s.CreateRestriction(222, -1005, 1, futureUntil, "{}")
	bot.checkExpired()
	r1, _ := s.ActiveRestriction(111, -1005)
	if r1 != nil {
		t.Error("expired restriction should be lifted")
	}
	r2, _ := s.ActiveRestriction(222, -1005)
	if r2 == nil {
		t.Error("future restriction should remain active")
	}
}

func TestCheckExpired_NoExpired(t *testing.T) {
	bot, _, s := newTestBot(t)
	futureUntil := time.Now().Add(24 * time.Hour)
	s.CreateRestriction(111, -1005, 1, futureUntil, "{}")
	bot.checkExpired()
	r, _ := s.ActiveRestriction(111, -1005)
	if r == nil {
		t.Error("restriction should remain active")
	}
}

func TestPermissionCache(t *testing.T) {
	c := &permissionCache{items: make(map[int64]cacheEntry)}
	val, ok := c.get(-1005)
	if ok || val {
		t.Error("empty cache should return false, false")
	}
	c.set(-1005, true)
	val, ok = c.get(-1005)
	if !ok || !val {
		t.Error("should return true, true after Set")
	}
}

func TestPermissionCache_Expiry(t *testing.T) {
	c := &permissionCache{items: make(map[int64]cacheEntry)}
	c.set(-1005, true)
	entry := c.items[-1005]
	entry.expiresAt = time.Now().Add(-1 * time.Minute)
	c.items[-1005] = entry
	_, ok := c.get(-1005)
	if ok {
		t.Error("expired cache entry should return false")
	}
}

func TestHandleUpdate_UnknownType(t *testing.T) {
	bot, _, _ := newTestBot(t)
	update := telegram.Update{UpdateID: 1}
	bot.handleUpdate(update)
}

func TestHandleUpdate_Concurrency(t *testing.T) {
	bot, _, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			defer func() { done <- true }()
			update := telegram.Update{
				UpdateID:    int64(i + 1),
				ChannelPost: &telegram.Message{MessageID: int64(100 + i), From: &telegram.User{ID: int64(200 + i%3), FirstName: "Admin"}, Chat: telegram.Chat{ID: -1005, Type: "channel"}},
			}
			bot.handleUpdate(update)
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestCmdStart_Concurrency(t *testing.T) {
	bot, _, s := newTestBot(t)
	setupUserWithChannel(t, s, 111, -1005, 2)
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			bot.cmdStart(111)
			bot.cmdMyChannel(111)
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestUpdate_JSONMarshal(t *testing.T) {
	update := telegram.Update{
		UpdateID: 123,
		ChannelPost: &telegram.Message{
			MessageID: 456,
			From:      &telegram.User{ID: 789, FirstName: "Test"},
			Chat:      telegram.Chat{ID: -100, Type: "channel"},
			Text:      "hello",
		},
	}
	data, err := json.Marshal(update)
	if err != nil {
		t.Fatal(err)
	}
	var u2 telegram.Update
	if err := json.Unmarshal(data, &u2); err != nil {
		t.Fatal(err)
	}
	if u2.ChannelPost == nil || u2.ChannelPost.From.ID != 789 {
		t.Error("ChannelPost.From mismatch")
	}
}
