package storage

import (
	"testing"
	"time"
)

func newTestSQLite(t *testing.T) *SQLite {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRegisterUser_New(t *testing.T) {
	s := newTestSQLite(t)

	us, err := s.RegisterUser(111)
	if err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}
	if us.UserID != 111 {
		t.Errorf("UserID = %d, want 111", us.UserID)
	}
	if us.DailyLimit != 2 {
		t.Errorf("DailyLimit = %d, want 2", us.DailyLimit)
	}
	if !us.IsActive {
		t.Errorf("IsActive = false, want true")
	}
	if us.ChannelID.Valid {
		t.Error("ChannelID should not be set for new user")
	}
}

func TestRegisterUser_Existing(t *testing.T) {
	s := newTestSQLite(t)

	_, _ = s.RegisterUser(111)
	us, err := s.RegisterUser(111)
	if err != nil {
		t.Fatalf("RegisterUser (2nd): %v", err)
	}
	if us.UserID != 111 {
		t.Errorf("UserID = %d, want 111", us.UserID)
	}
}

func TestGetUserSettings_NotFound(t *testing.T) {
	s := newTestSQLite(t)

	us, err := s.GetUserSettings(999)
	if err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}
	if us != nil {
		t.Error("expected nil for nonexistent user")
	}
}

func TestGetUserSettings_Found(t *testing.T) {
	s := newTestSQLite(t)

	s.RegisterUser(111)
	us, err := s.GetUserSettings(111)
	if err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}
	if us == nil {
		t.Fatal("expected settings, got nil")
	}
	if us.UserID != 111 {
		t.Errorf("UserID = %d, want 111", us.UserID)
	}
}

func TestSetUserChannel(t *testing.T) {
	s := newTestSQLite(t)

	s.RegisterUser(111)
	err := s.SetUserChannel(111, -1001234567890)
	if err != nil {
		t.Fatalf("SetUserChannel: %v", err)
	}

	us, _ := s.GetUserSettings(111)
	if !us.ChannelID.Valid || us.ChannelID.Int64 != -1001234567890 {
		t.Errorf("ChannelID = %v, want -1001234567890", us.ChannelID)
	}
}

func TestSetUserDailyLimit(t *testing.T) {
	s := newTestSQLite(t)

	s.RegisterUser(111)
	err := s.SetUserDailyLimit(111, 5)
	if err != nil {
		t.Fatalf("SetUserDailyLimit: %v", err)
	}

	us, _ := s.GetUserSettings(111)
	if us.DailyLimit != 5 {
		t.Errorf("DailyLimit = %d, want 5", us.DailyLimit)
	}
}

func TestSetUserActive(t *testing.T) {
	s := newTestSQLite(t)

	s.RegisterUser(111)
	err := s.SetUserActive(111, false)
	if err != nil {
		t.Fatalf("SetUserActive: %v", err)
	}

	us, _ := s.GetUserSettings(111)
	if us.IsActive {
		t.Error("IsActive = true, want false")
	}
}

func TestGetChannelSettings(t *testing.T) {
	s := newTestSQLite(t)

	s.RegisterUser(111)
	s.SetUserChannel(111, -100999)

	us, err := s.GetChannelSettings(-100999)
	if err != nil {
		t.Fatalf("GetChannelSettings: %v", err)
	}
	if us == nil {
		t.Fatal("expected settings, got nil")
	}
	if us.UserID != 111 {
		t.Errorf("UserID = %d, want 111", us.UserID)
	}
}

func TestGetChannelSettings_NotFound(t *testing.T) {
	s := newTestSQLite(t)

	us, err := s.GetChannelSettings(-100999)
	if err != nil {
		t.Fatalf("GetChannelSettings: %v", err)
	}
	if us != nil {
		t.Error("expected nil for nonexistent channel")
	}
}

func TestAddExemption(t *testing.T) {
	s := newTestSQLite(t)

	err := s.AddExemption(222, -100888)
	if err != nil {
		t.Fatalf("AddExemption: %v", err)
	}

	exempt, _ := s.IsExempt(222, -100888)
	if !exempt {
		t.Error("IsExempt = false, want true")
	}
}

func TestAddExemption_Duplicate(t *testing.T) {
	s := newTestSQLite(t)

	s.AddExemption(222, -100888)
	err := s.AddExemption(222, -100888)
	if err != nil {
		t.Fatalf("AddExemption duplicate should be ignored: %v", err)
	}
}

func TestRemoveExemption(t *testing.T) {
	s := newTestSQLite(t)

	s.AddExemption(222, -100888)
	err := s.RemoveExemption(222, -100888)
	if err != nil {
		t.Fatalf("RemoveExemption: %v", err)
	}

	exempt, _ := s.IsExempt(222, -100888)
	if exempt {
		t.Error("IsExempt = true, want false after removal")
	}
}

func TestListExemptions(t *testing.T) {
	s := newTestSQLite(t)

	s.AddExemption(111, -100777)
	s.AddExemption(222, -100777)
	s.AddExemption(333, -100888)

	ids, err := s.ListExemptions(-100777)
	if err != nil {
		t.Fatalf("ListExemptions: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len = %d, want 2", len(ids))
	}

	found := map[int64]bool{}
	for _, id := range ids {
		found[id] = true
	}
	if !found[111] || !found[222] {
		t.Errorf("unexpected ids: %v", ids)
	}
}

func TestListExemptions_Empty(t *testing.T) {
	s := newTestSQLite(t)

	ids, err := s.ListExemptions(-100777)
	if err != nil {
		t.Fatalf("ListExemptions: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("len = %d, want 0", len(ids))
	}
}

func TestLogMessageAndCountToday(t *testing.T) {
	s := newTestSQLite(t)

	count, err := s.LogMessage(333, -100666)
	if err != nil {
		t.Fatalf("LogMessage #1: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	count, err = s.LogMessage(333, -100666)
	if err != nil {
		t.Fatalf("LogMessage #2: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	count, err = s.LogMessage(444, -100666)
	if err != nil {
		t.Fatalf("LogMessage other admin: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 for other admin", count)
	}

	count, _ = s.CountToday(333, -100666)
	if count != 2 {
		t.Errorf("CountToday = %d, want 2", count)
	}
}

func TestCreateRestriction(t *testing.T) {
	s := newTestSQLite(t)
	until := time.Now().Add(24 * time.Hour)

	err := s.CreateRestriction(555, -100555, 1, until, `{"test":true}`)
	if err != nil {
		t.Fatalf("CreateRestriction: %v", err)
	}

	r, err := s.ActiveRestriction(555, -100555)
	if err != nil {
		t.Fatalf("ActiveRestriction: %v", err)
	}
	if r == nil {
		t.Fatal("expected restriction, got nil")
	}
	if r.Offense != 1 {
		t.Errorf("Offense = %d, want 1", r.Offense)
	}
	if r.AdminID != 555 {
		t.Errorf("AdminID = %d, want 555", r.AdminID)
	}
	if r.Permissions != `{"test":true}` {
		t.Errorf("Permissions = %s", r.Permissions)
	}
}

func TestActiveRestriction_NotFound(t *testing.T) {
	s := newTestSQLite(t)

	r, err := s.ActiveRestriction(999, -100999)
	if err != nil {
		t.Fatalf("ActiveRestriction: %v", err)
	}
	if r != nil {
		t.Error("expected nil for nonexistent restriction")
	}
}

func TestCreateRestriction_DefaultPermissions(t *testing.T) {
	s := newTestSQLite(t)
	until := time.Now().Add(24 * time.Hour)

	err := s.CreateRestriction(555, -100555, 1, until, "")
	if err != nil {
		t.Fatalf("CreateRestriction: %v", err)
	}

	r, _ := s.ActiveRestriction(555, -100555)
	if r.Permissions != "{}" {
		t.Errorf("Permissions = %s, want {}", r.Permissions)
	}
}

func TestLifetimeOffenses(t *testing.T) {
	s := newTestSQLite(t)
	until := time.Now().Add(24 * time.Hour)

	count, _ := s.LifetimeOffenses(666, -100666)
	if count != 0 {
		t.Errorf("initial offenses = %d, want 0", count)
	}

	s.CreateRestriction(666, -100666, 1, until, "{}")
	count, _ = s.LifetimeOffenses(666, -100666)
	if count != 1 {
		t.Errorf("offenses = %d, want 1", count)
	}

	s.CreateRestriction(666, -100666, 2, until.Add(48*time.Hour), "{}")
	count, _ = s.LifetimeOffenses(666, -100666)
	if count != 2 {
		t.Errorf("offenses = %d, want 2", count)
	}
}

func TestLiftRestriction(t *testing.T) {
	s := newTestSQLite(t)
	until := time.Now().Add(24 * time.Hour)

	s.CreateRestriction(777, -100777, 1, until, "{}")
	r, _ := s.ActiveRestriction(777, -100777)
	if r == nil {
		t.Fatal("restriction should exist")
	}

	err := s.LiftRestriction(777, -100777)
	if err != nil {
		t.Fatalf("LiftRestriction: %v", err)
	}

	r, _ = s.ActiveRestriction(777, -100777)
	if r != nil {
		t.Error("restriction should be inactive after lift")
	}
}

func TestLiftAllRestrictions(t *testing.T) {
	s := newTestSQLite(t)
	until := time.Now().Add(24 * time.Hour)

	s.CreateRestriction(111, -100888, 1, until, "{}")
	s.CreateRestriction(222, -100888, 1, until, "{}")
	s.CreateRestriction(333, -100999, 1, until, "{}")

	err := s.LiftAllRestrictions(-100888)
	if err != nil {
		t.Fatalf("LiftAllRestrictions: %v", err)
	}

	r1, _ := s.ActiveRestriction(111, -100888)
	r2, _ := s.ActiveRestriction(222, -100888)
	r3, _ := s.ActiveRestriction(333, -100999)

	if r1 != nil {
		t.Error("restriction 111 should be inactive")
	}
	if r2 != nil {
		t.Error("restriction 222 should be inactive")
	}
	if r3 == nil {
		t.Error("restriction 333 in other channel should remain active")
	}
}

func TestGetExpiredRestrictions(t *testing.T) {
	s := newTestSQLite(t)

	pastUntil := time.Now().Add(-24 * time.Hour)
	futureUntil := time.Now().Add(24 * time.Hour)

	s.CreateRestriction(111, -100777, 1, pastUntil, "{}")
	s.CreateRestriction(222, -100777, 1, futureUntil, "{}")

	expired, err := s.GetExpiredRestrictions()
	if err != nil {
		t.Fatalf("GetExpiredRestrictions: %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("len = %d, want 1", len(expired))
	}
	if expired[0].AdminID != 111 {
		t.Errorf("AdminID = %d, want 111", expired[0].AdminID)
	}
}

func TestGetExpiredRestrictions_LiftedNotIncluded(t *testing.T) {
	s := newTestSQLite(t)

	pastUntil := time.Now().Add(-24 * time.Hour)
	s.CreateRestriction(111, -100777, 1, pastUntil, "{}")
	s.LiftRestriction(111, -100777)

	expired, _ := s.GetExpiredRestrictions()
	if len(expired) != 0 {
		t.Errorf("len = %d, want 0 (lifted restriction should not appear)", len(expired))
	}
}

func TestGetAllActiveRestrictions(t *testing.T) {
	s := newTestSQLite(t)
	until := time.Now().Add(24 * time.Hour)

	s.CreateRestriction(111, -100666, 1, until, "{}")
	s.CreateRestriction(222, -100666, 2, until, "{}")
	s.CreateRestriction(333, -100555, 1, until, "{}")

	all, err := s.GetAllActiveRestrictions(-100666)
	if err != nil {
		t.Fatalf("GetAllActiveRestrictions: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("len = %d, want 2", len(all))
	}

	ids := map[int64]int{}
	for _, r := range all {
		ids[r.AdminID] = r.Offense
	}
	if ids[111] != 1 || ids[222] != 2 {
		t.Errorf("unexpected restrictions: %v", ids)
	}
}

func TestGetAllActiveRestrictions_LiftedExcluded(t *testing.T) {
	s := newTestSQLite(t)
	until := time.Now().Add(24 * time.Hour)

	s.CreateRestriction(111, -100666, 1, until, "{}")
	s.CreateRestriction(222, -100666, 1, until, "{}")
	s.LiftRestriction(111, -100666)

	all, _ := s.GetAllActiveRestrictions(-100666)
	if len(all) != 1 {
		t.Fatalf("len = %d, want 1", len(all))
	}
	if all[0].AdminID != 222 {
		t.Errorf("AdminID = %d, want 222", all[0].AdminID)
	}
}

func TestRestrictionTimeParsing(t *testing.T) {
	s := newTestSQLite(t)
	until := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	s.CreateRestriction(999, -100999, 3, until, "{}")

	r, _ := s.ActiveRestriction(999, -100999)
	if r == nil {
		t.Fatal("expected restriction")
	}
	if r.Until.Year() != 2026 || r.Until.Month() != 6 || r.Until.Day() != 15 {
		t.Errorf("Until = %v, want 2026-06-15", r.Until)
	}
	if r.Created.IsZero() {
		t.Error("Created should not be zero")
	}
}

func TestRegisterUser_IsActiveDefaultsTrue(t *testing.T) {
	s := newTestSQLite(t)
	us, _ := s.RegisterUser(123)
	if !us.IsActive {
		t.Error("new user should be active by default")
	}
}

func TestIsExempt_NotFound(t *testing.T) {
	s := newTestSQLite(t)
	exempt, err := s.IsExempt(999, -100999)
	if err != nil {
		t.Fatalf("IsExempt: %v", err)
	}
	if exempt {
		t.Error("IsExempt = true, want false")
	}
}

func TestChannelID_ValidAndNull(t *testing.T) {
	s := newTestSQLite(t)

	s.RegisterUser(111)
	us, _ := s.GetUserSettings(111)
	if us.ChannelID.Valid {
		t.Error("new user ChannelID.Valid should be false")
	}
	if us.ChannelID.Int64 != 0 {
		t.Errorf("ChannelID.Int64 should be 0, got %d", us.ChannelID.Int64)
	}

	s.SetUserChannel(111, -100999)
	us, _ = s.GetUserSettings(111)
	if !us.ChannelID.Valid {
		t.Error("ChannelID.Valid should be true after SetUserChannel")
	}
	if us.ChannelID.Int64 != -100999 {
		t.Errorf("ChannelID = %d, want -100999", us.ChannelID.Int64)
	}
}

func TestNew_InvalidPath(t *testing.T) {
	_, err := New("/nonexistent/path/to/nowhere/db.sqlite")
	if err == nil {
		t.Error("New with nonexistent directory should fail")
	}
}

func TestMultipleUsers_SeparateChannels(t *testing.T) {
	s := newTestSQLite(t)

	s.RegisterUser(111)
	s.RegisterUser(222)

	s.SetUserChannel(111, -100111)
	s.SetUserChannel(222, -100222)

	us1, _ := s.GetChannelSettings(-100111)
	us2, _ := s.GetChannelSettings(-100222)

	if us1.UserID != 111 {
		t.Errorf("channel -100111 belongs to user %d, want 111", us1.UserID)
	}
	if us2.UserID != 222 {
		t.Errorf("channel -100222 belongs to user %d, want 222", us2.UserID)
	}
}

func TestClose(t *testing.T) {
	s := newTestSQLite(t)
	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestDB_Version(t *testing.T) {
	s := newTestSQLite(t)
	var v string
	s.db.QueryRow("SELECT sqlite_version()").Scan(&v)
	if v == "" {
		t.Error("failed to get sqlite_version()")
	}
}
