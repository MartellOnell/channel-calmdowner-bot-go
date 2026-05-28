package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type UserSettings struct {
	UserID     int64
	ChannelID  sql.NullInt64
	DailyLimit int
	IsActive   bool
}

type Restriction struct {
	ID          int64
	AdminID     int64
	ChannelID   int64
	Permissions string
	Offense     int
	Created     time.Time
	Until       time.Time
	Active      bool
}

type SQLite struct {
	db *sql.DB
}

func New(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=5000")
	db.Exec("PRAGMA foreign_keys=ON")

	s := &SQLite{db: db}
	if err := s.initialize(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize schema: %w", err)
	}

	return s, nil
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

func (s *SQLite) DB() *sql.DB {
	return s.db
}

func (s *SQLite) initialize() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS user_settings (
			user_id INTEGER PRIMARY KEY,
			channel_id INTEGER,
			daily_limit INTEGER NOT NULL DEFAULT 2,
			is_active INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE IF NOT EXISTS exemptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			admin_id INTEGER NOT NULL,
			channel_id INTEGER NOT NULL,
			UNIQUE(admin_id, channel_id)
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			admin_id INTEGER NOT NULL,
			channel_id INTEGER NOT NULL,
			msg_date TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS restrictions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			admin_id INTEGER NOT NULL,
			channel_id INTEGER NOT NULL,
			permissions TEXT NOT NULL DEFAULT '{}',
			offense INTEGER NOT NULL DEFAULT 1,
			created TEXT NOT NULL,
			until TEXT NOT NULL,
			active INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX IF NOT EXISTS ix_messages_admin_channel_date ON messages(admin_id, channel_id, msg_date)`,
		`CREATE INDEX IF NOT EXISTS ix_restrictions_admin_channel_active ON restrictions(admin_id, channel_id, active)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("schema init: %w", err)
		}
	}
	return nil
}

func (s *SQLite) RegisterUser(userID int64) (*UserSettings, error) {
	existing, err := s.GetUserSettings(userID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	_, err = s.db.Exec("INSERT INTO user_settings (user_id) VALUES (?)", userID)
	if err != nil {
		return nil, fmt.Errorf("register user: %w", err)
	}
	return &UserSettings{UserID: userID, DailyLimit: 2, IsActive: true}, nil
}

func (s *SQLite) GetUserSettings(userID int64) (*UserSettings, error) {
	var us UserSettings
	err := s.db.QueryRow(
		"SELECT user_id, channel_id, daily_limit, is_active FROM user_settings WHERE user_id = ?",
		userID,
	).Scan(&us.UserID, &us.ChannelID, &us.DailyLimit, &us.IsActive)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user settings: %w", err)
	}
	return &us, nil
}

func (s *SQLite) GetChannelSettings(channelID int64) (*UserSettings, error) {
	var us UserSettings
	err := s.db.QueryRow(
		"SELECT user_id, channel_id, daily_limit, is_active FROM user_settings WHERE channel_id = ? LIMIT 1",
		channelID,
	).Scan(&us.UserID, &us.ChannelID, &us.DailyLimit, &us.IsActive)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get channel settings: %w", err)
	}
	return &us, nil
}

func (s *SQLite) SetUserChannel(userID, channelID int64) error {
	_, err := s.db.Exec(
		"UPDATE user_settings SET channel_id = ? WHERE user_id = ?",
		channelID, userID,
	)
	return err
}

func (s *SQLite) SetUserDailyLimit(userID int64, limit int) error {
	_, err := s.db.Exec(
		"UPDATE user_settings SET daily_limit = ? WHERE user_id = ?",
		limit, userID,
	)
	return err
}

func (s *SQLite) SetUserActive(userID int64, active bool) error {
	_, err := s.db.Exec(
		"UPDATE user_settings SET is_active = ? WHERE user_id = ?",
		active, userID,
	)
	return err
}

func (s *SQLite) AddExemption(adminID, channelID int64) error {
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO exemptions (admin_id, channel_id) VALUES (?, ?)",
		adminID, channelID,
	)
	return err
}

func (s *SQLite) RemoveExemption(adminID, channelID int64) error {
	_, err := s.db.Exec(
		"DELETE FROM exemptions WHERE admin_id = ? AND channel_id = ?",
		adminID, channelID,
	)
	return err
}

func (s *SQLite) ListExemptions(channelID int64) ([]int64, error) {
	rows, err := s.db.Query(
		"SELECT admin_id FROM exemptions WHERE channel_id = ?",
		channelID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *SQLite) IsExempt(adminID, channelID int64) (bool, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM exemptions WHERE admin_id = ? AND channel_id = ?",
		adminID, channelID,
	).Scan(&count)
	return count > 0, err
}

func (s *SQLite) LogMessage(adminID, channelID int64) (int, error) {
	today := time.Now().Format("2006-01-02")
	_, err := s.db.Exec(
		"INSERT INTO messages (admin_id, channel_id, msg_date) VALUES (?, ?, ?)",
		adminID, channelID, today,
	)
	if err != nil {
		return 0, fmt.Errorf("log message: %w", err)
	}
	return s.CountToday(adminID, channelID)
}

func (s *SQLite) CountToday(adminID, channelID int64) (int, error) {
	today := time.Now().Format("2006-01-02")
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM messages WHERE admin_id = ? AND channel_id = ? AND msg_date = ?",
		adminID, channelID, today,
	).Scan(&count)
	return count, err
}

func (s *SQLite) ActiveRestriction(adminID, channelID int64) (*Restriction, error) {
	var r Restriction
	var createdStr, untilStr string
	err := s.db.QueryRow(
		`SELECT id, admin_id, channel_id, permissions, offense, created, until, active
		 FROM restrictions WHERE admin_id = ? AND channel_id = ? AND active = 1 LIMIT 1`,
		adminID, channelID,
	).Scan(&r.ID, &r.AdminID, &r.ChannelID, &r.Permissions, &r.Offense, &createdStr, &untilStr, &r.Active)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Created, _ = time.Parse(time.RFC3339, createdStr)
	r.Until, _ = time.Parse(time.RFC3339, untilStr)
	return &r, nil
}

func (s *SQLite) LifetimeOffenses(adminID, channelID int64) (int, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM restrictions WHERE admin_id = ? AND channel_id = ?",
		adminID, channelID,
	).Scan(&count)
	return count, err
}

func (s *SQLite) CreateRestriction(adminID, channelID int64, offense int, until time.Time, permissions string) error {
	now := time.Now()
	if permissions == "" {
		permissions = "{}"
	}
	_, err := s.db.Exec(
		"INSERT INTO restrictions (admin_id, channel_id, permissions, offense, created, until, active) VALUES (?, ?, ?, ?, ?, ?, 1)",
		adminID, channelID, permissions, offense, now.Format(time.RFC3339), until.Format(time.RFC3339),
	)
	return err
}

func (s *SQLite) LiftRestriction(adminID, channelID int64) error {
	_, err := s.db.Exec(
		"UPDATE restrictions SET active = 0 WHERE admin_id = ? AND channel_id = ? AND active = 1",
		adminID, channelID,
	)
	return err
}

func (s *SQLite) LiftAllRestrictions(channelID int64) error {
	_, err := s.db.Exec(
		"UPDATE restrictions SET active = 0 WHERE channel_id = ? AND active = 1",
		channelID,
	)
	return err
}

func (s *SQLite) GetExpiredRestrictions() ([]Restriction, error) {
	now := time.Now().Format(time.RFC3339)
	rows, err := s.db.Query(
		`SELECT id, admin_id, channel_id, permissions, offense, created, until, active
		 FROM restrictions WHERE active = 1 AND until < ?`,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRestrictions(rows)
}

func (s *SQLite) GetAllActiveRestrictions(channelID int64) ([]Restriction, error) {
	rows, err := s.db.Query(
		`SELECT id, admin_id, channel_id, permissions, offense, created, until, active
		 FROM restrictions WHERE channel_id = ? AND active = 1`,
		channelID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRestrictions(rows)
}

func scanRestrictions(rows *sql.Rows) ([]Restriction, error) {
	var restrictions []Restriction
	for rows.Next() {
		var r Restriction
		var createdStr, untilStr string
		if err := rows.Scan(&r.ID, &r.AdminID, &r.ChannelID, &r.Permissions, &r.Offense, &createdStr, &untilStr, &r.Active); err != nil {
			return nil, err
		}
		r.Created, _ = time.Parse(time.RFC3339, createdStr)
		r.Until, _ = time.Parse(time.RFC3339, untilStr)
		restrictions = append(restrictions, r)
	}
	return restrictions, rows.Err()
}
