package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"channel-calmdowner-bot/internal/storage"
	"channel-calmdowner-bot/internal/telegram"
)

type cacheEntry struct {
	canRestrict bool
	expiresAt   time.Time
}

type permissionCache struct {
	mu    sync.RWMutex
	items map[int64]cacheEntry
}

func (c *permissionCache) get(channelID int64) (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.items[channelID]
	if !ok || time.Now().After(entry.expiresAt) {
		return false, false
	}
	return entry.canRestrict, true
}

func (c *permissionCache) set(channelID int64, canRestrict bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[channelID] = cacheEntry{
		canRestrict: canRestrict,
		expiresAt:   time.Now().Add(5 * time.Minute),
	}
}

type Bot struct {
	botID int64
	tg    telegram.API
	store *storage.SQLite
	cache permissionCache
}

func New(botID int64, tg telegram.API, store *storage.SQLite) *Bot {
	return &Bot{
		botID: botID,
		tg:    tg,
		store: store,
		cache: permissionCache{
			items: make(map[int64]cacheEntry),
		},
	}
}

func (b *Bot) StartPolling() {
	offset := int64(0)
	for {
		updates, err := b.tg.GetUpdates(offset, 30)
		if err != nil {
			log.Printf("[poll] error: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		for _, update := range updates {
			b.handleUpdate(update)
			offset = update.UpdateID + 1
		}
	}
}

func (b *Bot) StartPeriodicCheck(intervalSeconds int) {
	ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		b.checkExpired()
	}
}

func (b *Bot) HandleUpdate(update telegram.Update) {
	b.handleUpdate(update)
}

func (b *Bot) handleUpdate(update telegram.Update) {
	if update.ChannelPost != nil {
		b.handleChannelMessage(update.ChannelPost)
		return
	}
	if update.Message != nil {
		b.handleMessage(update.Message)
		return
	}
}

func (b *Bot) handleMessage(msg *telegram.Message) {
	if msg.From == nil {
		return
	}
	if msg.Text == "" || !strings.HasPrefix(msg.Text, "/") {
		return
	}

	parts := strings.Fields(msg.Text)
	cmd := parts[0]
	args := parts[1:]

	if i := strings.Index(cmd, "@"); i != -1 {
		cmd = cmd[:i]
	}

	chatID := msg.Chat.ID
	fromID := msg.From.ID

	var response string
	switch cmd {
	case "/start":
		response = b.cmdStart(fromID)
	case "/help":
		response = b.cmdHelp()
	case "/addchannel":
		response = b.cmdAddChannel(fromID, args)
	case "/mychannel":
		response = b.cmdMyChannel(fromID)
	case "/setlimit":
		response = b.cmdSetLimit(fromID, args)
	case "/unrestrict":
		response = b.cmdUnrestrict(fromID, args)
	case "/unrestrict_all":
		response = b.cmdUnrestrictAll(fromID)
	case "/status":
		response = b.cmdStatus(fromID)
	case "/admins":
		response = b.cmdAdmins(fromID)
	case "/exempt":
		response = b.cmdExempt(fromID, args)
	case "/exempt_remove":
		response = b.cmdExemptRemove(fromID, args)
	case "/exempt_list":
		response = b.cmdExemptList(fromID)
	case "/checkadmin":
		response = b.cmdCheckAdmin(fromID)
	default:
		return
	}

	if response != "" {
		if err := b.tg.SendMessage(chatID, response, msg.MessageID); err != nil {
			log.Printf("[send] error: %v", err)
		}
	}
}

func (b *Bot) requireSettings(userID int64) (*storage.UserSettings, string) {
	settings, err := b.store.GetUserSettings(userID)
	if err != nil {
		return nil, "Ошибка базы данных. Пожалуйста, попробуйте позже."
	}
	if settings == nil {
		return nil, "Вы не зарегистрированы. Используйте /start сначала."
	}
	if !settings.ChannelID.Valid {
		return nil, "Канал не привязан. Используйте /addchannel <channel_id> сначала."
	}
	return settings, ""
}

func (b *Bot) cmdStart(userID int64) string {
	settings, err := b.store.RegisterUser(userID)
	if err != nil {
		log.Printf("[start] register error: %v", err)
		return "Ошибка регистрации. Пожалуйста, попробуйте ещё раз."
	}

	channelStr := "не задан"
	if settings.ChannelID.Valid {
		channelStr = fmt.Sprintf("%d", settings.ChannelID.Int64)
	}

	status := "Активен"
	if !settings.IsActive {
		status = "Приостановлен"
	}

	return fmt.Sprintf(
		"Добро пожаловать! Channel Calmdowner Bot готов.\n"+
			"ID канала: %s\n"+
			"Дневной лимит: %d сообщений\n"+
			"Статус: %s\n\n"+
			"Используйте /help для списка команд.",
		channelStr, settings.DailyLimit, status,
	)
}

func (b *Bot) cmdHelp() string {
	return "Доступные команды:\n" +
		"/start — Регистрация и статус бота\n" +
		"/help — Показать эту справку\n" +
		"/addchannel <channel_id> — Привязать канал для мониторинга\n" +
		"/mychannel — Показать настройки канала\n" +
		"/setlimit <number> — Установить дневной лимит сообщений\n" +
		"/unrestrict <user_id> — Вручную снять ограничение\n" +
		"/unrestrict_all — Снять все ограничения в канале\n" +
		"/status — Показать активные ограничения\n" +
		"/admins — Список администраторов канала\n" +
		"/exempt <user_id> — Исключить админа из ограничений\n" +
		"/exempt_remove <user_id> — Убрать исключение\n" +
		"/exempt_list — Список исключённых админов\n" +
		"/checkadmin — Проверить права бота"
}

func (b *Bot) cmdAddChannel(userID int64, args []string) string {
	settings, err := b.store.GetUserSettings(userID)
	if err != nil {
		return "Ошибка базы данных."
	}
	if settings == nil {
		return "Вы не зарегистрированы. Используйте /start сначала."
	}

	if len(args) == 0 {
		return "Использование: /addchannel <channel_id>"
	}

	channelID, err := parseInt64(args[0])
	if err != nil {
		return "Неверный ID канала. Должен быть числом."
	}

	chat, err := b.tg.GetChat(channelID)
	if err != nil {
		return "Неверный ID канала. Бот не может получить доступ к этому чату. Убедитесь, что бот состоит в канале."
	}

	channelName := chat.Title
	if channelName == "" {
		channelName = fmt.Sprintf("%d", channelID)
	}

	member, err := b.tg.GetChatMember(channelID, b.botID)
	if err != nil {
		return "Не удалось проверить права бота в этом канале."
	}

	if member.Status != "administrator" && member.Status != "creator" {
		return "Бот должен быть администратором с правом 'Удаление сообщений'."
	}

	if member.Status == "administrator" && !member.CanDeleteMessages {
		return "Бот должен иметь право 'Удаление сообщений'."
	}

	if err := b.store.SetUserChannel(userID, channelID); err != nil {
		log.Printf("[addchannel] set channel error: %v", err)
		return "Ошибка базы данных. Пожалуйста, попробуйте ещё раз."
	}

	return fmt.Sprintf(
		"Канал привязан: %s (%d)\nБот будет отслеживать сообщения и соблюдать дневные лимиты.",
		channelName, channelID,
	)
}

func (b *Bot) cmdMyChannel(userID int64) string {
	settings, err := b.store.GetUserSettings(userID)
	if err != nil {
		return "Ошибка базы данных."
	}
	if settings == nil {
		return "Вы не зарегистрированы. Используйте /start сначала."
	}

	channelStr := "не задан"
	if settings.ChannelID.Valid {
		channelStr = fmt.Sprintf("%d", settings.ChannelID.Int64)
	}

	status := "Активен"
	if !settings.IsActive {
		status = "Приостановлен"
	}

	return fmt.Sprintf(
		"Ваши настройки:\n"+
			"ID канала: %s\n"+
			"Дневной лимит: %d сообщений\n"+
			"Статус: %s",
		channelStr, settings.DailyLimit, status,
	)
}

func (b *Bot) cmdSetLimit(userID int64, args []string) string {
	settings, errMsg := b.requireSettings(userID)
	if errMsg != "" {
		return errMsg
	}

	if len(args) == 0 {
		return "Использование: /setlimit <число>"
	}

	limit, err := parseInt(args[0])
	if err != nil || limit < 1 {
		return "Неверный лимит. Должен быть положительным числом (минимум 1)."
	}

	if err := b.store.SetUserDailyLimit(userID, limit); err != nil {
		log.Printf("[setlimit] error: %v", err)
		return "Ошибка базы данных."
	}

	_ = settings
	return fmt.Sprintf("Дневной лимит установлен: %d сообщений.", limit)
}

func (b *Bot) cmdUnrestrict(userID int64, args []string) string {
	settings, errMsg := b.requireSettings(userID)
	if errMsg != "" {
		return errMsg
	}

	if len(args) == 0 {
		return "Использование: /unrestrict <user_id>"
	}

	adminID, err := parseInt64(args[0])
	if err != nil {
		return "Неверный ID пользователя."
	}

	if err := b.store.LiftRestriction(adminID, settings.ChannelID.Int64); err != nil {
		log.Printf("[unrestrict] error: %v", err)
		return "Ошибка базы данных."
	}

	return fmt.Sprintf("Ограничение снято для пользователя %d.", adminID)
}

func (b *Bot) cmdUnrestrictAll(userID int64) string {
	settings, errMsg := b.requireSettings(userID)
	if errMsg != "" {
		return errMsg
	}

	if err := b.store.LiftAllRestrictions(settings.ChannelID.Int64); err != nil {
		log.Printf("[unrestrict_all] error: %v", err)
		return "Ошибка базы данных."
	}

	return "Все ограничения сняты для этого канала."
}

func (b *Bot) cmdStatus(userID int64) string {
	settings, errMsg := b.requireSettings(userID)
	if errMsg != "" {
		return errMsg
	}

	restrictions, err := b.store.GetAllActiveRestrictions(settings.ChannelID.Int64)
	if err != nil {
		log.Printf("[status] error: %v", err)
		return "Ошибка базы данных."
	}

	if len(restrictions) == 0 {
		return "Нет активных ограничений."
	}

	channelID := settings.ChannelID.Int64
	chat, err := b.tg.GetChat(channelID)
	channelName := fmt.Sprintf("%d", channelID)
	if err == nil && chat.Title != "" {
		channelName = chat.Title
	}

	lines := []string{fmt.Sprintf("Активные ограничения в %s (%d):", channelName, channelID)}
	for _, r := range restrictions {
		until := r.Until.Format("2006-01-02 15:04")
		lines = append(lines, fmt.Sprintf("Админ ID: %d — Нарушение #%d — До %s", r.AdminID, r.Offense, until))
	}

	return strings.Join(lines, "\n")
}

func (b *Bot) cmdAdmins(userID int64) string {
	settings, errMsg := b.requireSettings(userID)
	if errMsg != "" {
		return errMsg
	}

	channelID := settings.ChannelID.Int64

	admins, err := b.tg.GetChatAdministrators(channelID)
	if err != nil {
		return "Не удалось получить список администраторов канала. Убедитесь, что бот имеет права администратора."
	}

	chat, err := b.tg.GetChat(channelID)
	channelName := fmt.Sprintf("%d", channelID)
	if err == nil && chat.Title != "" {
		channelName = chat.Title
	}

	lines := []string{fmt.Sprintf("Администраторы в %s (%d):", channelName, channelID)}
	idx := 0
	for _, admin := range admins {
		if admin.Status != "creator" && admin.Status != "administrator" {
			continue
		}
		idx++

		name := admin.User.FirstName
		if admin.User.LastName != "" {
			name += " " + admin.User.LastName
		}

		username := ""
		if admin.User.Username != "" {
			username = "@" + admin.User.Username
		}

		adminID := admin.User.ID

		isExempt, _ := b.store.IsExempt(adminID, channelID)
		restriction, _ := b.store.ActiveRestriction(adminID, channelID)

		var status string
		switch {
		case isExempt:
			status = "Исключён"
		case restriction != nil:
			until := restriction.Until.Format("2006-01-02 15:04")
			status = fmt.Sprintf("Ограничен (до %s)", until)
		case admin.Status == "creator":
			status = "Владелец"
		default:
			status = "Активен"
		}

		lines = append(lines, fmt.Sprintf("%d. %s %s (ID: %d) - %s", idx, name, username, adminID, status))
	}

	return strings.Join(lines, "\n")
}

func (b *Bot) cmdExempt(userID int64, args []string) string {
	settings, errMsg := b.requireSettings(userID)
	if errMsg != "" {
		return errMsg
	}

	if len(args) == 0 {
		return "Использование: /exempt <user_id>"
	}

	adminID, err := parseInt64(args[0])
	if err != nil {
		return "Неверный ID пользователя."
	}

	if err := b.store.AddExemption(adminID, settings.ChannelID.Int64); err != nil {
		log.Printf("[exempt] error: %v", err)
		return "Ошибка базы данных."
	}

	return fmt.Sprintf("Пользователь %d теперь исключён из ограничений.", adminID)
}

func (b *Bot) cmdExemptRemove(userID int64, args []string) string {
	settings, errMsg := b.requireSettings(userID)
	if errMsg != "" {
		return errMsg
	}

	if len(args) == 0 {
		return "Использование: /exempt_remove <user_id>"
	}

	adminID, err := parseInt64(args[0])
	if err != nil {
		return "Неверный ID пользователя."
	}

	if err := b.store.RemoveExemption(adminID, settings.ChannelID.Int64); err != nil {
		log.Printf("[exempt_remove] error: %v", err)
		return "Ошибка базы данных."
	}

	return fmt.Sprintf("Исключение убрано для пользователя %d.", adminID)
}

func (b *Bot) cmdExemptList(userID int64) string {
	settings, errMsg := b.requireSettings(userID)
	if errMsg != "" {
		return errMsg
	}

	ids, err := b.store.ListExemptions(settings.ChannelID.Int64)
	if err != nil {
		log.Printf("[exempt_list] error: %v", err)
		return "Ошибка базы данных."
	}

	if len(ids) == 0 {
		return "Нет исключений в этом канале."
	}

	lines := []string{"Исключённые администраторы:"}
	for _, id := range ids {
		name := fmt.Sprintf("%d", id)
		member, err := b.tg.GetChatMember(settings.ChannelID.Int64, id)
		if err == nil {
			if member.User.FirstName != "" {
				name = member.User.FirstName
			}
			if member.User.Username != "" {
				name += " (@" + member.User.Username + ")"
			}
			name += fmt.Sprintf(" [ID: %d]", id)
		}
		lines = append(lines, "- "+name)
	}

	return strings.Join(lines, "\n")
}

func (b *Bot) cmdCheckAdmin(userID int64) string {
	settings, errMsg := b.requireSettings(userID)
	if errMsg != "" {
		return errMsg
	}

	member, err := b.tg.GetChatMember(settings.ChannelID.Int64, b.botID)
	if err != nil {
		return "Не удалось проверить права бота в этом канале. Убедитесь, что бот состоит в канале."
	}

	if member.Status == "creator" {
		return "Бот — владелец канала, имеет полные права."
	}

	if member.Status != "administrator" {
		return "Бот не является администратором. Пожалуйста, назначьте бота администратором с правом 'Удаление сообщений'."
	}

	var lines []string
	lines = append(lines, "Бот является администратором со следующими правами:")

	printPerm := func(name string, value bool) {
		icon := "\u2705"
		if !value {
			icon = "\u274c"
		}
		lines = append(lines, fmt.Sprintf("%s %s", icon, name))
	}

	printPerm("Удаление сообщений", member.CanDeleteMessages)
	printPerm("Управление чатом", member.CanManageChat)
	printPerm("Публикация сообщений", member.CanPostMessages)
	printPerm("Редактирование сообщений", member.CanEditMessages)
	printPerm("Ограничение участников", member.CanRestrictMembers)
	printPerm("Повышение участников", member.CanPromoteMembers)
	printPerm("Изменение информации", member.CanChangeInfo)
	printPerm("Приглашение пользователей", member.CanInviteUsers)
	printPerm("Закрепление сообщений", member.CanPinMessages)

	if !member.CanDeleteMessages {
		lines = append(lines, "\nПРЕДУПРЕЖДЕНИЕ: Бот НЕ имеет права 'Удаление сообщений'. Он не сможет удалять нарушающие лимит сообщения.")
	}

	return strings.Join(lines, "\n")
}

func (b *Bot) handleChannelMessage(msg *telegram.Message) {
	if msg.SenderChat != nil && msg.SenderChat.ID == b.botID {
		return
	}
	if msg.From == nil {
		return
	}

	channelID := msg.Chat.ID
	adminID := msg.From.ID

	canRestrict, ok := b.cache.get(channelID)
	if !ok {
		member, err := b.tg.GetChatMember(channelID, b.botID)
		if err != nil {
			return
		}
		canRestrict = member.Status == "creator" ||
			(member.Status == "administrator" && member.CanDeleteMessages)
		b.cache.set(channelID, canRestrict)
	}

	if !canRestrict {
		return
	}

	settings, err := b.store.GetChannelSettings(channelID)
	if err != nil || settings == nil || !settings.IsActive {
		return
	}

	restriction, _ := b.store.ActiveRestriction(adminID, channelID)
	if restriction != nil {
		if err := b.tg.DeleteMessage(channelID, msg.MessageID); err != nil {
			log.Printf("[delete] error: %v", err)
		}
		return
	}

	exempt, _ := b.store.IsExempt(adminID, channelID)
	if exempt {
		return
	}

	count, err := b.store.LogMessage(adminID, channelID)
	if err != nil {
		log.Printf("[logmsg] error: %v", err)
		return
	}

	if count > settings.DailyLimit {
		b.applyRestriction(adminID, channelID)
		if err := b.tg.DeleteMessage(channelID, msg.MessageID); err != nil {
			log.Printf("[delete] error: %v", err)
		}
	}
}

func (b *Bot) applyRestriction(adminID, channelID int64) {
	offenses, _ := b.store.LifetimeOffenses(adminID, channelID)
	newOffense := offenses + 1

	until := time.Now().Add(restrictionDuration(newOffense))

	permissions := "{}"
	member, err := b.tg.GetChatMember(channelID, adminID)
	if err == nil && member.Status == "administrator" {
		perms := map[string]bool{
			"can_delete_messages":  member.CanDeleteMessages,
			"can_manage_chat":      member.CanManageChat,
			"can_post_messages":    member.CanPostMessages,
			"can_edit_messages":    member.CanEditMessages,
			"can_restrict_members": member.CanRestrictMembers,
			"can_promote_members":  member.CanPromoteMembers,
			"can_change_info":      member.CanChangeInfo,
			"can_invite_users":     member.CanInviteUsers,
			"can_pin_messages":     member.CanPinMessages,
		}
		if permsJSON, err := json.Marshal(perms); err == nil {
			permissions = string(permsJSON)
		}
	}

	if err := b.store.CreateRestriction(adminID, channelID, newOffense, until, permissions); err != nil {
		log.Printf("[restrict] db error: %v", err)
		return
	}

	log.Printf("Restricted admin %d in channel %d (offense #%d, until %s)",
		adminID, channelID, newOffense, until.Format("2006-01-02 15:04"))
}

func (b *Bot) checkExpired() {
	restrictions, err := b.store.GetExpiredRestrictions()
	if err != nil {
		log.Printf("[check_expired] query error: %v", err)
		return
	}

	for _, r := range restrictions {
		if err := b.store.LiftRestriction(r.AdminID, r.ChannelID); err != nil {
			log.Printf("[check_expired] lift error (admin %d, channel %d): %v", r.AdminID, r.ChannelID, err)
		} else {
			log.Printf("Lifted expired restriction for admin %d in channel %d (offense #%d)", r.AdminID, r.ChannelID, r.Offense)
		}
	}
}

func restrictionDuration(offense int) time.Duration {
	if offense <= 0 {
		return 24 * time.Hour
	}
	const maxDays = int64(365 * 1000)
	var days int64 = 1
	for i := 1; i < offense; i++ {
		if days > maxDays/2 {
			return time.Duration(math.MaxInt64)
		}
		days *= 2
	}
	d := time.Duration(days) * 24 * time.Hour
	if d <= 0 {
		return time.Duration(math.MaxInt64)
	}
	return d
}

func parseInt64(s string) (int64, error) {
	var result int64
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}
