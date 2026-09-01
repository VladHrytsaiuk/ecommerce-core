package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	support "github.com/VladHrytsaiuk/ecommerce-core/internal/support/domain"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type ticket support.Ticket

func (ticket) TableName() string { return "support_tickets" }

type message support.Message

func (message) TableName() string { return "support_messages" }

func (r *Repository) Create(ctx context.Context, t support.Ticket, m support.Message) (*support.Ticket, error) {
	t.ID, m.ID, m.TicketID = uuid.New(), uuid.New(), t.ID
	err := transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		if err := tx.Create((*ticket)(&t)).Error; err != nil {
			return err
		}
		return tx.Create((*message)(&m)).Error
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) List(ctx context.Context, status string, limit, offset int) ([]support.Ticket, int64, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, support.ErrInvalidTicket
	}
	query := r.database(ctx).Model((*ticket)(nil))
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ticket
	if err := query.Order("updated_at DESC, id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	result := make([]support.Ticket, len(rows))
	for i := range rows {
		result[i] = support.Ticket(rows[i])
	}
	return result, total, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (*support.Ticket, []support.Message, error) {
	var row ticket
	if err := r.database(ctx).First(&row, "id = ?", id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, support.ErrTicketNotFound
	} else if err != nil {
		return nil, nil, err
	}
	var messageRows []message
	if err := r.database(ctx).Where("ticket_id = ?", id).Order("created_at ASC, id ASC").Find(&messageRows).Error; err != nil {
		return nil, nil, err
	}
	messages := make([]support.Message, len(messageRows))
	for i := range messageRows {
		messages[i] = support.Message(messageRows[i])
	}
	value := support.Ticket(row)
	return &value, messages, nil
}

func (r *Repository) AddAgentMessageAndSetPending(ctx context.Context, ticketID, agentID uuid.UUID, body string) (*support.Ticket, error) {
	if _, err := transaction.FromContext(ctx); err != nil {
		return nil, err
	}
	var row ticket
	if err := r.database(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id = ?", ticketID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, support.ErrTicketNotFound
	} else if err != nil {
		return nil, err
	}
	if !support.CanTransition(row.Status, support.StatusPendingCustomer) {
		return nil, support.ErrInvalidTicket
	}
	if err := r.database(ctx).Create((*message)(&support.Message{ID: uuid.New(), TicketID: ticketID, SenderType: "agent", SenderID: &agentID, Body: body})).Error; err != nil {
		return nil, err
	}
	if err := r.database(ctx).Model(&row).Updates(map[string]any{"status": support.StatusPendingCustomer, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
		return nil, err
	}
	row.Status = support.StatusPendingCustomer
	return (*support.Ticket)(&row), nil
}

func (r *Repository) TransitionStatus(ctx context.Context, ticketID uuid.UUID, target string) (*support.Ticket, error) {
	if _, err := transaction.FromContext(ctx); err != nil {
		return nil, err
	}
	var row ticket
	if err := r.database(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id = ?", ticketID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, support.ErrTicketNotFound
	} else if err != nil {
		return nil, err
	}
	if !support.CanTransition(row.Status, target) {
		return nil, support.ErrInvalidTicket
	}
	if row.Status != target {
		if err := r.database(ctx).Model(&row).Updates(map[string]any{"status": target, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
			return nil, err
		}
		row.Status = target
	}
	return (*support.Ticket)(&row), nil
}

// AddCustomerMessage deliberately scopes both the lookup and insert to the JWT
// subject. Guest follow-ups need a future signed capability token, not a guessable UUID.
func (r *Repository) AddCustomerMessage(ctx context.Context, ticketID, customerID uuid.UUID, body string) error {
	// Locking makes the terminal-status check and insert one indivisible action:
	// a customer reply cannot race a manager closing the same ticket.
	return transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		var row ticket
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND customer_id = ?", ticketID, customerID).First(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return support.ErrTicketNotFound
		} else if err != nil {
			return err
		}
		if row.Status == support.StatusClosed {
			return support.ErrMessageForbidden
		}
		return tx.WithContext(ctx).Create((*message)(&support.Message{ID: uuid.New(), TicketID: ticketID, SenderType: "customer", SenderID: &customerID, Body: body})).Error
	})
}

func (r *Repository) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

var _ support.Repository = (*Repository)(nil)
