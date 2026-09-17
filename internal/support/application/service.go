package application

import (
	"context"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	notifications "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	support "github.com/VladHrytsaiuk/ecommerce-core/internal/support/domain"
)

type Service struct {
	repository support.Repository
	spam       support.SpamProtector
	customers  support.CustomerEmailReader
	tx         support.TransactionManager
	scheduler  notifications.NotificationScheduler
}

func NewService(repository support.Repository, spam support.SpamProtector, customers support.CustomerEmailReader) *Service {
	return &Service{repository: repository, spam: spam, customers: customers}
}

func (s *Service) WithAdminWorkflow(tx support.TransactionManager, scheduler notifications.NotificationScheduler) *Service {
	s.tx, s.scheduler = tx, scheduler
	return s
}

func (s *Service) ListTickets(ctx context.Context, status string, page, limit int) ([]support.Ticket, int64, error) {
	if s == nil || s.repository == nil || page < 1 || limit < 1 || limit > 100 {
		return nil, 0, support.ErrInvalidTicket
	}
	return s.repository.List(ctx, strings.TrimSpace(status), limit, (page-1)*limit)
}
func (s *Service) GetTicket(ctx context.Context, id uuid.UUID) (*support.Ticket, []support.Message, error) {
	if s == nil || s.repository == nil || id == uuid.Nil {
		return nil, nil, support.ErrInvalidTicket
	}
	return s.repository.Get(ctx, id)
}

func (s *Service) ReplyAsAgent(ctx context.Context, ticketID, agentID uuid.UUID, body string) (*support.Ticket, error) {
	if s == nil || s.repository == nil || s.tx == nil || s.scheduler == nil || ticketID == uuid.Nil || agentID == uuid.Nil {
		return nil, support.ErrInvalidTicket
	}
	body, err := validMessageBody(body, support.MaxAgentReplyRunes)
	if err != nil {
		return nil, err
	}
	var result *support.Ticket
	err = s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		ticket, err := s.repository.AddAgentMessageAndSetPending(txCtx, ticketID, agentID, body)
		if err != nil {
			return err
		}
		if err := // support_tickets records no locale, so the store's is used.
			s.scheduler.ScheduleEmail(txCtx, "support_agent_reply", "", ticket.Email, struct {
				TicketID    uuid.UUID `json:"ticket_id"`
				Subject     string    `json:"subject"`
				MessageBody string    `json:"message_body"`
			}{ticket.ID, ticket.Subject, body}); err != nil {
			return err
		}
		result = ticket
		return nil
	})
	return result, err
}

func (s *Service) ChangeStatus(ctx context.Context, ticketID, agentID uuid.UUID, target string) (*support.Ticket, error) {
	if s == nil || s.repository == nil || s.tx == nil || ticketID == uuid.Nil || agentID == uuid.Nil {
		return nil, support.ErrInvalidTicket
	}
	var result *support.Ticket
	err := s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		result, err = s.repository.TransitionStatus(txCtx, ticketID, strings.TrimSpace(target))
		return err
	})
	return result, err
}

func (s *Service) CreateTicket(ctx context.Context, customerID *uuid.UUID, email, subject, body, ip string) (*support.Ticket, error) {
	if s == nil || s.repository == nil || s.spam == nil {
		return nil, support.ErrInvalidTicket
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if customerID != nil && *customerID != uuid.Nil && email == "" && s.customers != nil {
		var err error
		email, err = s.customers.EmailForCustomer(ctx, *customerID)
		if err != nil {
			return nil, err
		}
		email = strings.ToLower(strings.TrimSpace(email))
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || strings.TrimSpace(subject) == "" || utf8.RuneCountInString(subject) > support.MaxSubjectRunes {
		return nil, support.ErrInvalidTicket
	}
	// The opening message is bounded here rather than only at the transport,
	// so a caller that is not the HTTP handler cannot write an unbounded body.
	body, err = validMessageBody(body, support.MaxCustomerMessageRunes)
	if err != nil {
		return nil, err
	}
	if err := s.spam.Check(ctx, ip, email, body); err != nil {
		return nil, err
	}
	return s.repository.Create(ctx, support.Ticket{CustomerID: customerID, Email: email, Subject: strings.TrimSpace(subject), Status: support.StatusNew}, support.Message{SenderType: "customer", SenderID: customerID, Body: body})
}

func (s *Service) AddCustomerMessage(ctx context.Context, ticketID, customerID uuid.UUID, body, ip, email string) error {
	if s == nil || s.repository == nil || s.spam == nil || ticketID == uuid.Nil || customerID == uuid.Nil {
		return support.ErrMessageForbidden
	}
	body, err := validMessageBody(body, support.MaxCustomerMessageRunes)
	if err != nil {
		return err
	}
	if err := s.spam.Check(ctx, ip, email, body); err != nil {
		return err
	}
	return s.repository.AddCustomerMessage(ctx, ticketID, customerID, body)
}

// validMessageBody trims and bounds one message. It returns the trimmed body so
// no caller can validate one string and store another — the mismatch that let a
// body pass a length check and reach the table with different content.
func validMessageBody(body string, limit int) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > limit {
		return "", support.ErrInvalidTicket
	}
	return body, nil
}
