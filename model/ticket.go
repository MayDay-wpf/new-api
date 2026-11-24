package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	TicketStatusOpen   = "open"
	TicketStatusClosed = "closed"

	TicketSenderRoleUser  = "user"
	TicketSenderRoleAdmin = "admin"

	TicketMaxTitleLength   = 200
	TicketMaxContentLength = 64 * 1024 // 64 KB of HTML is already generous for a support thread
)

// Ticket 工单
type Ticket struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId    int    `json:"user_id" gorm:"not null;index"`
	Title     string `json:"title" gorm:"type:varchar(200);not null"`
	Status    string `json:"status" gorm:"type:varchar(16);not null;default:'open';index"`
	CreatedAt int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt int64  `json:"updated_at" gorm:"bigint"`
	ClosedAt  *int64 `json:"closed_at" gorm:"bigint"`

	// Read markers — the last time each side opened the thread. A ticket is
	// "unread" for a role when its latest message is newer than that role's
	// last_read_at. Zero value means never opened.
	UserLastReadAt  int64 `json:"-" gorm:"bigint;default:0"`
	AdminLastReadAt int64 `json:"-" gorm:"bigint;default:0"`

	// Joined fields (not persisted)
	Username      string `json:"username,omitempty" gorm:"-"`
	DisplayName   string `json:"display_name,omitempty" gorm:"-"`
	MessageCount  int64  `json:"message_count" gorm:"-"`
	LastMessageAt int64  `json:"last_message_at" gorm:"-"`
	// Convenience flag for list views — true when the *other* side wrote the
	// latest message after this role last opened the ticket.
	Unread bool `json:"unread" gorm:"-"`
}

func (Ticket) TableName() string {
	return "tickets"
}

// TicketMessage 工单消息
type TicketMessage struct {
	Id         int    `json:"id" gorm:"primaryKey;autoIncrement"`
	TicketId   int    `json:"ticket_id" gorm:"not null;index"`
	SenderId   int    `json:"sender_id" gorm:"not null"`
	SenderRole string `json:"sender_role" gorm:"type:varchar(8);not null"` // "user" | "admin"
	Content    string `json:"content" gorm:"type:text;not null"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint"`

	// Joined fields (not persisted)
	SenderName string `json:"sender_name,omitempty" gorm:"-"`
}

func (TicketMessage) TableName() string {
	return "ticket_messages"
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

func ticketBaseQuery() *gorm.DB {
	return DB.Model(&Ticket{}).
		Select("tickets.*, users.username, users.display_name").
		Joins("LEFT JOIN users ON users.id = tickets.user_id")
}

// GetTicketById fetches a single ticket with owner info.
func GetTicketById(id int) (*Ticket, error) {
	var ticket Ticket
	err := ticketBaseQuery().Where("tickets.id = ?", id).First(&ticket).Error
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

// GetTicketMessages returns all messages of a ticket in chronological order.
func GetTicketMessages(ticketId int) ([]*TicketMessage, error) {
	var messages []*TicketMessage
	err := DB.Model(&TicketMessage{}).
		Select("ticket_messages.*, users.username AS sender_name").
		Joins("LEFT JOIN users ON users.id = ticket_messages.sender_id").
		Where("ticket_messages.ticket_id = ?", ticketId).
		Order("ticket_messages.created_at ASC, ticket_messages.id ASC").
		Find(&messages).Error
	return messages, err
}

// CreateTicket inserts a ticket and its first message in one transaction.
func CreateTicket(userId int, title string, content string) (*Ticket, error) {
	if len(title) == 0 || len(title) > TicketMaxTitleLength {
		return nil, common.NewMessage("Ticket title is required and must be under {{max}} characters", map[string]any{"max": TicketMaxTitleLength})
	}
	if len(content) == 0 {
		return nil, common.NewMessage("Ticket content is required")
	}
	if len(content) > TicketMaxContentLength {
		return nil, common.NewMessage("Ticket content is too long")
	}

	now := time.Now().Unix()
	ticket := &Ticket{
		UserId:    userId,
		Title:     title,
		Status:    TicketStatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(ticket).Error; err != nil {
			return err
		}
		msg := &TicketMessage{
			TicketId:   ticket.Id,
			SenderId:   userId,
			SenderRole: TicketSenderRoleUser,
			Content:    content,
			CreatedAt:  now,
		}
		return tx.Create(msg).Error
	})
	if err != nil {
		return nil, err
	}
	return ticket, nil
}

// AddTicketMessage appends a message and bumps updated_at. When `reopen` is
// true (admin reply on a closed ticket), the ticket goes back to open.
func AddTicketMessage(ticketId int, senderId int, senderRole string, content string, reopen bool) (*TicketMessage, error) {
	if len(content) == 0 {
		return nil, common.NewMessage("Message content is required")
	}
	if len(content) > TicketMaxContentLength {
		return nil, common.NewMessage("Message content is too long")
	}

	now := time.Now().Unix()
	msg := &TicketMessage{
		TicketId:   ticketId,
		SenderId:   senderId,
		SenderRole: senderRole,
		Content:    content,
		CreatedAt:  now,
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(msg).Error; err != nil {
			return err
		}
		updates := map[string]any{"updated_at": now}
		if reopen {
			updates["status"] = TicketStatusOpen
			updates["closed_at"] = nil
		}
		return tx.Model(&Ticket{}).Where("id = ?", ticketId).Updates(updates).Error
	})
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// CloseTicket marks a ticket closed. Only the owner or an admin should call this.
func CloseTicket(id int, userId int, isAdmin bool) error {
	var ticket Ticket
	if err := DB.First(&ticket, "id = ?", id).Error; err != nil {
		return err
	}
	if !isAdmin && ticket.UserId != userId {
		return errors.New("permission denied")
	}
	if ticket.Status == TicketStatusClosed {
		return nil
	}
	now := time.Now().Unix()
	return DB.Model(&Ticket{}).Where("id = ?", id).Updates(map[string]any{
		"status":     TicketStatusClosed,
		"closed_at":  now,
		"updated_at": now,
	}).Error
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// MarkTicketRead updates the last_read_at for one side. `role` is either
// TicketSenderRoleUser or TicketSenderRoleAdmin.
func MarkTicketRead(id int, role string) error {
	column := "user_last_read_at"
	if role == TicketSenderRoleAdmin {
		column = "admin_last_read_at"
	}
	return DB.Model(&Ticket{}).Where("id = ?", id).Update(column, time.Now().Unix()).Error
}

// CountUnreadTickets returns how many tickets have a newer message than the
// role's last_read_at — for the sidebar badge.
func CountUnreadTickets(userId int, isAdmin bool) (int64, error) {
	var count int64
	query := DB.Model(&Ticket{})
	if isAdmin {
		// Admin sees everything: unread = any ticket whose last message is
		// newer than admin_last_read_at.
		query = query.Where(
			"updated_at > admin_last_read_at AND EXISTS (SELECT 1 FROM ticket_messages WHERE ticket_messages.ticket_id = tickets.id AND ticket_messages.sender_role = ?)",
			TicketSenderRoleUser,
		)
	} else {
		query = query.Where("user_id = ?", userId).Where(
			"updated_at > user_last_read_at AND EXISTS (SELECT 1 FROM ticket_messages WHERE ticket_messages.ticket_id = tickets.id AND ticket_messages.sender_role = ?)",
			TicketSenderRoleAdmin,
		)
	}
	err := query.Count(&count).Error
	return count, err
}

// GetUserTickets lists a user's tickets, newest first.
func GetUserTickets(userId int, page, pageSize int) ([]*Ticket, int64, error) {
	var tickets []*Ticket
	var total int64

	tx := DB.Model(&Ticket{}).Where("user_id = ?", userId)
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := ticketBaseQuery().
		Where("tickets.user_id = ?", userId).
		Order("tickets.updated_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&tickets).Error
	if err != nil {
		return nil, 0, err
	}
	fillTicketStats(tickets)
	unreadFlags := ticketUnreadFlags(ticketIdList(tickets), TicketSenderRoleAdmin)
	for _, t := range tickets {
		t.Unread = unreadFlags[t.Id] && t.LastMessageAt > t.UserLastReadAt
	}
	return tickets, total, nil
}

// ticketUnreadFlags returns a set of ticket ids that have at least one message
// from `senderRole` — one query for the whole batch.
func ticketUnreadFlags(ticketIds []int, senderRole string) map[int]bool {
	if len(ticketIds) == 0 {
		return nil
	}
	var ids []int
	if err := DB.Model(&TicketMessage{}).
		Where("ticket_id IN ? AND sender_role = ?", ticketIds, senderRole).
		Distinct("ticket_id").
		Pluck("ticket_id", &ids).Error; err != nil {
		return nil
	}
	flags := make(map[int]bool, len(ids))
	for _, id := range ids {
		flags[id] = true
	}
	return flags
}

func ticketIdList(tickets []*Ticket) []int {
	ids := make([]int, len(tickets))
	for i, t := range tickets {
		ids[i] = t.Id
	}
	return ids
}

// GetAllTickets lists every ticket (admin view), newest first.
func GetAllTickets(status string, keyword string, page, pageSize int) ([]*Ticket, int64, error) {
	var tickets []*Ticket
	var total int64

	query := DB.Model(&Ticket{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where(
			"title LIKE ? OR id IN (SELECT id FROM tickets WHERE CAST(id AS TEXT) LIKE ?)",
			like, like,
		)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	q := ticketBaseQuery()
	if status != "" {
		q = q.Where("tickets.status = ?", status)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("tickets.title LIKE ? OR CAST(tickets.id AS TEXT) LIKE ?", like, like)
	}
	err := q.Order("tickets.updated_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&tickets).Error
	if err != nil {
		return nil, 0, err
	}
	fillTicketStats(tickets)
	unreadFlags := ticketUnreadFlags(ticketIdList(tickets), TicketSenderRoleUser)
	for _, t := range tickets {
		t.Unread = unreadFlags[t.Id] && t.LastMessageAt > t.AdminLastReadAt
	}
	return tickets, total, nil
}
func fillTicketStats(tickets []*Ticket) {
	if len(tickets) == 0 {
		return
	}
	ids := make([]int, len(tickets))
	for i, t := range tickets {
		ids[i] = t.Id
	}
	type statRow struct {
		TicketId      int
		MessageCount  int64
		LastMessageAt int64
	}
	var rows []statRow
	err := DB.Model(&TicketMessage{}).
		Select("ticket_id, COUNT(*) AS message_count, MAX(created_at) AS last_message_at").
		Where("ticket_id IN ?", ids).
		Group("ticket_id").
		Scan(&rows).Error
	if err != nil {
		return
	}
	stats := make(map[int]statRow, len(rows))
	for _, r := range rows {
		stats[r.TicketId] = r
	}
	for _, t := range tickets {
		if s, ok := stats[t.Id]; ok {
			t.MessageCount = s.MessageCount
			t.LastMessageAt = s.LastMessageAt
		}
	}
}
