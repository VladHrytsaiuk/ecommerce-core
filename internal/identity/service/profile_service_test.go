package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

func TestProfileServiceValidatesPolicyBeforePersisting(t *testing.T) {
	repo := &profileRepositoryFake{}
	policy := domain.ProfilePolicy{SchemaVersion: 1, Fields: []domain.ProfileFieldConfig{
		{Key: "birth_date", Type: domain.ProfileFieldDate, Required: true, CustomerWritable: true},
		{Key: "gender", Type: domain.ProfileFieldEnum, CustomerWritable: true, AllowedValues: []string{"female", "male", "undisclosed"}},
		{Key: "risk_score", Type: domain.ProfileFieldNumber, CustomerWritable: false},
	}}
	service := NewProfileService(policy, repo)
	userID := uuid.New()

	profile, err := service.UpdateProfile(context.Background(), userID, []byte(`{"birth_date":"1990-01-31","gender":"female"}`))
	if err != nil || profile.UserID != userID || repo.upserts != 1 {
		t.Fatalf("UpdateProfile() = (%+v, %v), upserts=%d", profile, err, repo.upserts)
	}
	if _, err := service.UpdateProfile(context.Background(), userID, []byte(`{"birth_date":"1990-01-31","risk_score":10}`)); !errors.Is(err, domain.ErrInvalidProfile) {
		t.Fatalf("UpdateProfile() error = %v, want invalid profile", err)
	}
	if repo.upserts != 1 {
		t.Fatalf("invalid profile persisted, upserts=%d", repo.upserts)
	}
}

func TestProfileServicePatchPreservesServerManagedFields(t *testing.T) {
	userID := uuid.New()
	repo := &profileRepositoryFake{current: &domain.Profile{UserID: userID, SchemaVersion: 1, Attributes: map[string]json.RawMessage{
		"tshirt_size": json.RawMessage(`"L"`),
		"risk_score":  json.RawMessage(`10`),
	}}}
	service := NewProfileService(domain.ProfilePolicy{SchemaVersion: 1, Fields: []domain.ProfileFieldConfig{
		{Key: "tshirt_size", Type: domain.ProfileFieldString, CustomerWritable: true},
		{Key: "risk_score", Type: domain.ProfileFieldNumber, CustomerWritable: false},
	}}, repo)

	profile, err := service.UpdateProfile(context.Background(), userID, []byte(`{"tshirt_size":"XL"}`))
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if string(profile.Attributes["tshirt_size"]) != `"XL"` || string(profile.Attributes["risk_score"]) != "10" {
		t.Fatalf("merged profile = %v", profile.Attributes)
	}
	if _, err := service.UpdateProfile(context.Background(), userID, []byte(`{"risk_score":20}`)); !errors.Is(err, domain.ErrInvalidProfile) {
		t.Fatalf("UpdateProfile() error = %v, want invalid profile", err)
	}
}

func TestProfileService_UpdateProfile_Concurrency(t *testing.T) {
	userID := uuid.New()
	repo := newConcurrentProfileRepository(&domain.Profile{UserID: userID, SchemaVersion: 1, Revision: 1, Attributes: map[string]json.RawMessage{
		"first_name": json.RawMessage(`"Ada"`),
		"last_name":  json.RawMessage(`"Lovelace"`),
	}})
	service := NewProfileService(domain.ProfilePolicy{SchemaVersion: 1, Fields: []domain.ProfileFieldConfig{
		{Key: "first_name", Type: domain.ProfileFieldString, CustomerWritable: true},
		{Key: "last_name", Type: domain.ProfileFieldString, CustomerWritable: true},
	}}, repo)

	start := make(chan struct{})
	errorsByPatch := make(chan error, 2)
	for _, patch := range [][]byte{[]byte(`{"first_name":"Grace"}`), []byte(`{"last_name":"Hopper"}`)} {
		go func(patch []byte) {
			<-start
			_, err := service.UpdateProfile(context.Background(), userID, patch)
			errorsByPatch <- err
		}(patch)
	}
	close(start)

	successes, conflicts := 0, 0
	for range 2 {
		err := <-errorsByPatch
		if err == nil {
			successes++
		} else if errors.Is(err, domain.ErrProfileConflict) {
			conflicts++
		} else {
			t.Fatalf("UpdateProfile() error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d, want one each", successes, conflicts)
	}
	if repo.current.Revision != 2 {
		t.Fatalf("revision=%d, want 2", repo.current.Revision)
	}
}

type profileRepositoryFake struct {
	upserts int
	current *domain.Profile
}

func (r *profileRepositoryFake) FindByUserID(context.Context, uuid.UUID) (*domain.Profile, error) {
	if r.current == nil {
		return nil, domain.ErrProfileNotFound
	}
	return cloneProfile(r.current), nil
}

func (r *profileRepositoryFake) Upsert(_ context.Context, profile domain.Profile) (*domain.Profile, error) {
	if r.current == nil {
		if profile.Revision != 0 {
			return nil, domain.ErrProfileConflict
		}
		profile.Revision = 1
	} else {
		if profile.Revision != r.current.Revision {
			return nil, domain.ErrProfileConflict
		}
		profile.Revision++
	}
	r.upserts++
	r.current = cloneProfile(&profile)
	return cloneProfile(r.current), nil
}

type concurrentProfileRepository struct {
	mu          sync.Mutex
	current     *domain.Profile
	findStarted chan struct{}
	finds       int
}

func newConcurrentProfileRepository(profile *domain.Profile) *concurrentProfileRepository {
	return &concurrentProfileRepository{current: cloneProfile(profile), findStarted: make(chan struct{})}
}

func (r *concurrentProfileRepository) FindByUserID(context.Context, uuid.UUID) (*domain.Profile, error) {
	r.mu.Lock()
	snapshot := cloneProfile(r.current)
	r.finds++
	waitForConcurrentRead := r.finds <= 2
	if r.finds == 2 {
		close(r.findStarted)
	}
	r.mu.Unlock()
	if waitForConcurrentRead {
		<-r.findStarted
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return snapshot, nil
}

func (r *concurrentProfileRepository) Upsert(_ context.Context, profile domain.Profile) (*domain.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if profile.Revision != r.current.Revision {
		return nil, domain.ErrProfileConflict
	}
	profile.Revision++
	r.current = cloneProfile(&profile)
	return cloneProfile(r.current), nil
}

func cloneProfile(profile *domain.Profile) *domain.Profile {
	clone := *profile
	clone.Attributes = make(map[string]json.RawMessage, len(profile.Attributes))
	for key, value := range profile.Attributes {
		clone.Attributes[key] = append(json.RawMessage(nil), value...)
	}
	return &clone
}
