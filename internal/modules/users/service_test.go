package users

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
)

type nopLogger struct{}

func (nopLogger) Info(string)                                       {}
func (nopLogger) Warn(string)                                       {}
func (nopLogger) Error(string)                                      {}
func (nopLogger) Debug(string)                                      {}
func (l nopLogger) WithField(string, interface{}) logger.Logger     { return l }
func (l nopLogger) WithError(error) logger.Logger                   { return l }
func (l nopLogger) WithFields(map[string]interface{}) logger.Logger { return l }

type fakeRepo struct {
	user          *User
	getErr        error
	updateErr     error
	softDeleteErr error

	updatedName     string
	updatedPicture  *string
	softDeletedID   uuid.UUID
	softDeleteCalls int
}

func (f *fakeRepo) GetByID(_ context.Context, _ uuid.UUID) (*User, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.user == nil {
		return nil, apperrors.ErrNotFound
	}
	return f.user, nil
}

func (f *fakeRepo) UpdateProfile(_ context.Context, _ uuid.UUID, name string, picture *string) (*User, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	f.updatedName = name
	f.updatedPicture = picture
	updated := *f.user
	updated.Name = name
	if picture != nil {
		updated.Picture = *picture
	}
	return &updated, nil
}

func (f *fakeRepo) SoftDelete(_ context.Context, id uuid.UUID) error {
	f.softDeleteCalls++
	f.softDeletedID = id
	return f.softDeleteErr
}

type fakeRevoker struct {
	err       error
	calls     int
	lastToken string
}

func (f *fakeRevoker) Logout(_ context.Context, token string) error {
	f.calls++
	f.lastToken = token
	return f.err
}

func existingUser() *User {
	return &User{
		ID:        uuid.New(),
		GoogleID:  "google-123",
		Email:     "user@example.com",
		Name:      "Old Name",
		Picture:   "https://example.com/old.jpg",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func strPtr(s string) *string { return &s }

func TestUpdateProfile_Success(t *testing.T) {
	user := existingUser()
	repo := &fakeRepo{user: user}
	svc := NewService(repo, &fakeRevoker{}, nopLogger{})

	updated, err := svc.UpdateProfile(context.Background(), user.ID, UpdateUserRequest{
		Name:    "  New Name  ",
		Picture: strPtr("https://example.com/new.jpg"),
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if updated.Name != "New Name" {
		t.Errorf("expected trimmed name %q, got %q", "New Name", updated.Name)
	}
	if repo.updatedPicture == nil || *repo.updatedPicture != "https://example.com/new.jpg" {
		t.Errorf("expected picture to be updated")
	}
}

func TestUpdateProfile_KeepsPictureWhenOmitted(t *testing.T) {
	user := existingUser()
	repo := &fakeRepo{user: user}
	svc := NewService(repo, &fakeRevoker{}, nopLogger{})

	_, err := svc.UpdateProfile(context.Background(), user.ID, UpdateUserRequest{Name: "New Name"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if repo.updatedPicture == nil || *repo.updatedPicture != user.Picture {
		t.Errorf("expected existing picture to be preserved")
	}
}

func TestUpdateProfile_ClearsPictureWithEmptyString(t *testing.T) {
	user := existingUser()
	repo := &fakeRepo{user: user}
	svc := NewService(repo, &fakeRevoker{}, nopLogger{})

	_, err := svc.UpdateProfile(context.Background(), user.ID, UpdateUserRequest{
		Name:    "New Name",
		Picture: strPtr(""),
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if repo.updatedPicture == nil || *repo.updatedPicture != "" {
		t.Errorf("expected picture to be cleared")
	}
}

func TestUpdateProfile_RejectsEmptyName(t *testing.T) {
	user := existingUser()
	svc := NewService(&fakeRepo{user: user}, &fakeRevoker{}, nopLogger{})

	_, err := svc.UpdateProfile(context.Background(), user.ID, UpdateUserRequest{Name: "   "})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestUpdateProfile_RejectsLongName(t *testing.T) {
	user := existingUser()
	svc := NewService(&fakeRepo{user: user}, &fakeRevoker{}, nopLogger{})

	_, err := svc.UpdateProfile(context.Background(), user.ID, UpdateUserRequest{
		Name: strings.Repeat("a", maxNameLength+1),
	})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestUpdateProfile_RejectsInvalidPictureURL(t *testing.T) {
	user := existingUser()
	svc := NewService(&fakeRepo{user: user}, &fakeRevoker{}, nopLogger{})

	for _, candidate := range []string{"not-a-url", "ftp://example.com/a.jpg", "javascript:alert(1)"} {
		_, err := svc.UpdateProfile(context.Background(), user.ID, UpdateUserRequest{
			Name:    "New Name",
			Picture: strPtr(candidate),
		})
		if !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput for %q, got %v", candidate, err)
		}
	}
}

func TestUpdateProfile_RejectsLongPictureURL(t *testing.T) {
	user := existingUser()
	svc := NewService(&fakeRepo{user: user}, &fakeRevoker{}, nopLogger{})

	_, err := svc.UpdateProfile(context.Background(), user.ID, UpdateUserRequest{
		Name:    "New Name",
		Picture: strPtr("https://example.com/" + strings.Repeat("a", maxPictureLength)),
	})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestUpdateProfile_NotFound(t *testing.T) {
	svc := NewService(&fakeRepo{getErr: apperrors.ErrNotFound}, &fakeRevoker{}, nopLogger{})

	_, err := svc.UpdateProfile(context.Background(), uuid.New(), UpdateUserRequest{Name: "New Name"})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteAccount_SoftDeletesAndRevokesToken(t *testing.T) {
	user := existingUser()
	repo := &fakeRepo{user: user}
	revoker := &fakeRevoker{}
	svc := NewService(repo, revoker, nopLogger{})

	if err := svc.DeleteAccount(context.Background(), user.ID, "active-token"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if repo.softDeleteCalls != 1 || repo.softDeletedID != user.ID {
		t.Errorf("expected soft delete for user %s", user.ID)
	}
	if revoker.calls != 1 || revoker.lastToken != "active-token" {
		t.Errorf("expected active token to be revoked")
	}
}

func TestDeleteAccount_SucceedsWhenRevocationFails(t *testing.T) {
	user := existingUser()
	repo := &fakeRepo{user: user}
	svc := NewService(repo, &fakeRevoker{err: errors.New("blacklist down")}, nopLogger{})

	if err := svc.DeleteAccount(context.Background(), user.ID, "active-token"); err != nil {
		t.Fatalf("expected delete to succeed despite revocation failure, got %v", err)
	}
}

func TestDeleteAccount_PropagatesRepoError(t *testing.T) {
	user := existingUser()
	repo := &fakeRepo{user: user, softDeleteErr: errors.New("db down")}
	revoker := &fakeRevoker{}
	svc := NewService(repo, revoker, nopLogger{})

	if err := svc.DeleteAccount(context.Background(), user.ID, "active-token"); err == nil {
		t.Fatal("expected error to propagate")
	}
	if revoker.calls != 0 {
		t.Errorf("token must not be revoked when delete fails")
	}
}
