package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	badgesDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	reviewsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/domain"
	seoDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/seo/domain"
)

type contentTxMarker struct{}

// contentTransaction stamps the context it hands to the unit of work. The
// production publisher resolves its transaction from the context, so checking
// for the stamp proves the audit event shares the mutation's transaction
// rather than merely running during it.
type contentTransaction struct{ rolledBack bool }

func (t *contentTransaction) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	err := fn(context.WithValue(ctx, contentTxMarker{}, true))
	if err != nil {
		t.rolledBack = true
	}
	return err
}

type contentPublisher struct {
	actions           []string
	insideTransaction bool
	err               error
}

func (p *contentPublisher) Publish(ctx context.Context, event events.DomainEvent) error {
	if p.err != nil {
		return p.err
	}
	p.actions = append(p.actions, event.Topic())
	p.insideTransaction, _ = ctx.Value(contentTxMarker{}).(bool)
	return nil
}

type contentAllow struct{}

func (contentAllow) Require(context.Context, uuid.UUID, string) error { return nil }

type contentDeny struct{ permission string }

func (d *contentDeny) Require(_ context.Context, _ uuid.UUID, permission string) error {
	d.permission = permission
	return adminDomain.ErrPermissionDenied
}

type badgesFake struct{ created, updated, deleted, assigned, removed int }

func (b *badgesFake) Get(context.Context, uuid.UUID) (*badgesDomain.Badge, error) {
	return &badgesDomain.Badge{ID: uuid.New(), Slug: "sale"}, nil
}
func (b *badgesFake) List(context.Context) ([]badgesDomain.Badge, error) { return nil, nil }
func (b *badgesFake) Create(_ context.Context, cmd badgesDomain.CreateCommand) (*badgesDomain.Badge, error) {
	b.created++
	return &badgesDomain.Badge{ID: uuid.New(), Slug: cmd.Slug}, nil
}
func (b *badgesFake) Update(_ context.Context, id uuid.UUID, cmd badgesDomain.UpdateCommand) (*badgesDomain.Badge, error) {
	b.updated++
	return &badgesDomain.Badge{ID: id, Slug: cmd.Slug}, nil
}
func (b *badgesFake) Delete(context.Context, uuid.UUID) error { b.deleted++; return nil }
func (b *badgesFake) AssignProduct(context.Context, uuid.UUID, uuid.UUID) error {
	b.assigned++
	return nil
}
func (b *badgesFake) RemoveProduct(context.Context, uuid.UUID, uuid.UUID) error {
	b.removed++
	return nil
}

type reviewsFake struct{ moderated, deleted int }

func (r *reviewsFake) Create(context.Context, reviewsDomain.CreateCommand) (*reviewsDomain.Review, error) {
	return nil, nil
}
func (r *reviewsFake) ListApproved(context.Context, uuid.UUID) ([]reviewsDomain.Review, error) {
	return nil, nil
}
func (r *reviewsFake) SetStatus(_ context.Context, id uuid.UUID, status reviewsDomain.Status) (*reviewsDomain.Review, error) {
	r.moderated++
	return &reviewsDomain.Review{ID: id, Status: status}, nil
}
func (r *reviewsFake) Delete(context.Context, uuid.UUID) error { r.deleted++; return nil }

type seoFake struct{ upserted, deleted int }

func (s *seoFake) Get(context.Context, string, uuid.UUID, string) (*seoDomain.Metadata, error) {
	return &seoDomain.Metadata{Title: "old"}, nil
}
func (s *seoFake) Upsert(_ context.Context, cmd seoDomain.UpsertCommand) (*seoDomain.Metadata, error) {
	s.upserted++
	return &seoDomain.Metadata{ResourceType: cmd.ResourceType, ResourceID: cmd.ResourceID, Title: cmd.Title}, nil
}
func (s *seoFake) Delete(context.Context, string, uuid.UUID, string) error { s.deleted++; return nil }

func newContentFacade(t *testing.T, authorizer adminDomain.Authorizer, tx TransactionManager, publisher events.TransactionalEventPublisher) *ContentAdminFacade {
	t.Helper()
	facade, err := NewContentAdminFacade(authorizer, tx, publisher)
	if err != nil {
		t.Fatalf("NewContentAdminFacade() error = %v", err)
	}
	return facade.WithBadges(&badgesFake{}).WithReviews(&reviewsFake{}).WithSEO(&seoFake{})
}

func TestContentMutationsAuditInsideTheSameTransaction(t *testing.T) {
	actor := uuid.New()
	for name, mutate := range map[string]func(*ContentAdminFacade) error{
		"create badge": func(f *ContentAdminFacade) error {
			_, err := f.CreateBadge(context.Background(), CatalogCommand{ActorUserID: actor}, badgesDomain.CreateCommand{Slug: "sale"})
			return err
		},
		"update badge": func(f *ContentAdminFacade) error {
			_, err := f.UpdateBadge(context.Background(), CatalogCommand{ActorUserID: actor}, uuid.New(), badgesDomain.UpdateCommand{Slug: "sale"})
			return err
		},
		"delete badge": func(f *ContentAdminFacade) error {
			return f.DeleteBadge(context.Background(), CatalogCommand{ActorUserID: actor}, uuid.New())
		},
		"assign badge": func(f *ContentAdminFacade) error {
			return f.AssignBadge(context.Background(), CatalogCommand{ActorUserID: actor}, uuid.New(), uuid.New())
		},
		"remove badge": func(f *ContentAdminFacade) error {
			return f.RemoveBadge(context.Background(), CatalogCommand{ActorUserID: actor}, uuid.New(), uuid.New())
		},
		"moderate review": func(f *ContentAdminFacade) error {
			_, err := f.ModerateReview(context.Background(), CatalogCommand{ActorUserID: actor}, uuid.New(), reviewsDomain.Status("approved"))
			return err
		},
		"delete review": func(f *ContentAdminFacade) error {
			return f.DeleteReview(context.Background(), CatalogCommand{ActorUserID: actor}, uuid.New())
		},
		"upsert seo": func(f *ContentAdminFacade) error {
			_, err := f.UpsertSEO(context.Background(), CatalogCommand{ActorUserID: actor}, seoDomain.UpsertCommand{ResourceType: "product", ResourceID: uuid.New(), Locale: "en"})
			return err
		},
		"delete seo": func(f *ContentAdminFacade) error {
			return f.DeleteSEO(context.Background(), CatalogCommand{ActorUserID: actor}, "product", uuid.New(), "en")
		},
	} {
		t.Run(name, func(t *testing.T) {
			publisher := &contentPublisher{}
			facade := newContentFacade(t, contentAllow{}, &contentTransaction{}, publisher)

			if err := mutate(facade); err != nil {
				t.Fatalf("mutation error = %v", err)
			}
			// An unaudited permission-protected mutation is the forensic bypass
			// router.go refuses to expose, so every one of these must record.
			if len(publisher.actions) != 1 || !publisher.insideTransaction {
				t.Fatalf("audit events = %d, inside transaction = %t; want 1 and true", len(publisher.actions), publisher.insideTransaction)
			}
		})
	}
}

func TestContentMutationRollsBackWhenAuditFails(t *testing.T) {
	transaction := &contentTransaction{}
	facade := newContentFacade(t, contentAllow{}, transaction, &contentPublisher{err: errors.New("outbox unavailable")})

	if _, err := facade.CreateBadge(context.Background(), CatalogCommand{ActorUserID: uuid.New()}, badgesDomain.CreateCommand{Slug: "sale"}); err == nil {
		t.Fatal("CreateBadge() error = nil, want the audit failure to fail the mutation")
	}
	if !transaction.rolledBack {
		t.Fatal("transaction committed despite the audit event failing")
	}
}

func TestContentMutationsRequireTheirOwnPermission(t *testing.T) {
	actor := uuid.New()
	for name, testCase := range map[string]struct {
		mutate func(*ContentAdminFacade) error
		want   string
	}{
		"badges": {want: PermissionBadgesWrite, mutate: func(f *ContentAdminFacade) error {
			_, err := f.CreateBadge(context.Background(), CatalogCommand{ActorUserID: actor}, badgesDomain.CreateCommand{Slug: "sale"})
			return err
		}},
		"reviews": {want: PermissionReviewsWrite, mutate: func(f *ContentAdminFacade) error {
			return f.DeleteReview(context.Background(), CatalogCommand{ActorUserID: actor}, uuid.New())
		}},
		"seo": {want: PermissionSEOWrite, mutate: func(f *ContentAdminFacade) error {
			return f.DeleteSEO(context.Background(), CatalogCommand{ActorUserID: actor}, "product", uuid.New(), "en")
		}},
	} {
		t.Run(name, func(t *testing.T) {
			authorizer := &contentDeny{}
			publisher := &contentPublisher{}
			facade := newContentFacade(t, authorizer, &contentTransaction{}, publisher)

			if err := testCase.mutate(facade); !errors.Is(err, adminDomain.ErrPermissionDenied) {
				t.Fatalf("mutation error = %v, want permission denied", err)
			}
			// Each module carries its own permission; a badges editor must not
			// inherit the ability to delete reviews.
			if authorizer.permission != testCase.want {
				t.Fatalf("checked permission = %q, want %q", authorizer.permission, testCase.want)
			}
			if len(publisher.actions) != 0 {
				t.Fatal("a denied request still produced an audit event")
			}
		})
	}
}

func TestContentFacadeRefusesADisabledModule(t *testing.T) {
	facade, err := NewContentAdminFacade(contentAllow{}, &contentTransaction{}, &contentPublisher{})
	if err != nil {
		t.Fatalf("NewContentAdminFacade() error = %v", err)
	}
	// Only badges are enabled here, so the other two must refuse rather than
	// dereference a nil service.
	facade.WithBadges(&badgesFake{})

	if !facade.HasBadges() || facade.HasReviews() || facade.HasSEO() {
		t.Fatalf("module availability = %t/%t/%t, want true/false/false", facade.HasBadges(), facade.HasReviews(), facade.HasSEO())
	}
	if err := facade.DeleteReview(context.Background(), CatalogCommand{ActorUserID: uuid.New()}, uuid.New()); err == nil {
		t.Fatal("DeleteReview() error = nil, want refusal for a disabled module")
	}
}

func TestContentFacadeRejectsAnonymousActor(t *testing.T) {
	facade := newContentFacade(t, contentAllow{}, &contentTransaction{}, &contentPublisher{})

	// uuid.Nil would otherwise be audited as a real administrator's action.
	if err := facade.DeleteBadge(context.Background(), CatalogCommand{}, uuid.New()); !errors.Is(err, adminDomain.ErrNotAdmin) {
		t.Fatalf("DeleteBadge() error = %v, want ErrNotAdmin", err)
	}
}
