package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		baseURL: fmt.Sprintf("https://api.telegram.org/bot%s/", token),
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type APIResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	Description string          `json:"description,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
}

type Update struct {
	UpdateID    int64    `json:"update_id"`
	Message     *Message `json:"message,omitempty"`
	ChannelPost *Message `json:"channel_post,omitempty"`
}

type Message struct {
	MessageID  int64  `json:"message_id"`
	From       *User  `json:"from,omitempty"`
	SenderChat *Chat  `json:"sender_chat,omitempty"`
	Chat       Chat   `json:"chat"`
	Date       int64  `json:"date"`
	Text       string `json:"text,omitempty"`
}

type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type Chat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}

type API interface {
	GetUpdates(offset int64, timeout int) ([]Update, error)
	SendMessage(chatID int64, text string, replyToMsgID int64) error
	GetChat(chatID int64) (*Chat, error)
	GetChatMember(chatID, userID int64) (*ChatMember, error)
	GetChatAdministrators(chatID int64) ([]ChatMember, error)
	DeleteMessage(chatID, messageID int64) error
}

type ChatMember struct {
	User              User   `json:"user"`
	Status            string `json:"status"`
	CanDeleteMessages bool   `json:"can_delete_messages,omitempty"`
	CanManageChat     bool   `json:"can_manage_chat,omitempty"`
	CanPostMessages   bool   `json:"can_post_messages,omitempty"`
	CanEditMessages   bool   `json:"can_edit_messages,omitempty"`
	CanRestrictMembers bool  `json:"can_restrict_members,omitempty"`
	CanPromoteMembers bool   `json:"can_promote_members,omitempty"`
	CanChangeInfo     bool   `json:"can_change_info,omitempty"`
	CanInviteUsers    bool   `json:"can_invite_users,omitempty"`
	CanPinMessages    bool   `json:"can_pin_messages,omitempty"`
	IsAnonymous       bool   `json:"is_anonymous,omitempty"`
	CustomTitle       string `json:"custom_title,omitempty"`
}

func (c *Client) call(method string, params any, result any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}

	resp, err := c.http.Post(c.baseURL+method, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if !apiResp.OK {
		if apiResp.Description != "" {
			return fmt.Errorf("telegram api error: %s (code %d)", apiResp.Description, apiResp.ErrorCode)
		}
		return fmt.Errorf("telegram api error: code %d", apiResp.ErrorCode)
	}

	if result != nil && len(apiResp.Result) > 0 {
		if err := json.Unmarshal(apiResp.Result, result); err != nil {
			return fmt.Errorf("decode result: %w", err)
		}
	}

	return nil
}

func (c *Client) GetUpdates(offset int64, timeout int) ([]Update, error) {
	params := map[string]any{
		"offset":  offset,
		"timeout": timeout,
		"limit":   100,
	}
	var updates []Update
	if err := c.call("getUpdates", params, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (c *Client) SendMessage(chatID int64, text string, replyToMsgID int64) error {
	params := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if replyToMsgID != 0 {
		params["reply_to_message_id"] = replyToMsgID
	}
	return c.call("sendMessage", params, nil)
}

func (c *Client) GetChat(chatID int64) (*Chat, error) {
	params := map[string]any{
		"chat_id": chatID,
	}
	var chat Chat
	if err := c.call("getChat", params, &chat); err != nil {
		return nil, err
	}
	return &chat, nil
}

func (c *Client) GetChatMember(chatID, userID int64) (*ChatMember, error) {
	params := map[string]any{
		"chat_id": chatID,
		"user_id": userID,
	}
	var member ChatMember
	if err := c.call("getChatMember", params, &member); err != nil {
		return nil, err
	}
	return &member, nil
}

func (c *Client) GetChatAdministrators(chatID int64) ([]ChatMember, error) {
	params := map[string]any{
		"chat_id": chatID,
	}
	var members []ChatMember
	if err := c.call("getChatAdministrators", params, &members); err != nil {
		return nil, err
	}
	return members, nil
}

func (c *Client) DeleteMessage(chatID, messageID int64) error {
	params := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
	}
	return c.call("deleteMessage", params, nil)
}
