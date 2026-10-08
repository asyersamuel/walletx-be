package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

func TestLocalReadReceiptValidation(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	require.NoError(t, err)
	owner := uuid.New()
	ref, err := store.SaveReceipt(context.Background(), owner, "image/png", "receipt.png", []byte("\x89PNG\r\n\x1a\n"))
	require.NoError(t, err)
	name, err := receiptPath(context.Background(), owner, ref)
	require.NoError(t, err)
	for _, image := range [][]byte{
		nil, []byte("not an image"), make([]byte, domain.MaxReceiptBytes+1),
		append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, domain.MaxReceiptBytes-8)...),
	} {
		require.NoError(t, os.WriteFile(filepath.Join(store.directory, name), image, 0600))
		got, err := store.ReadReceipt(context.Background(), owner, ref)
		if len(image) == domain.MaxReceiptBytes {
			require.NoError(t, err)
			require.Equal(t, image, got)
		} else {
			require.ErrorIs(t, err, apperrors.ErrInternal)
			require.Nil(t, got)
		}
	}
	_, err = store.ReadReceipt(nil, owner, ref)
	require.ErrorIs(t, err, apperrors.ErrInvalidInput)
	_, err = store.ReadReceipt(context.Background(), uuid.Nil, ref)
	require.ErrorIs(t, err, apperrors.ErrInvalidInput)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.ReadReceipt(ctx, owner, ref)
	require.ErrorIs(t, err, context.Canceled)
}

func TestLocal_SaveSurvivesAdapterRestartAndDeletes(t *testing.T) {
	directory := t.TempDir()
	store, err := NewLocal(directory)
	require.NoError(t, err)
	owner := uuid.New()
	image := []byte("\x89PNG\r\n\x1a\n")
	reference, err := store.SaveReceipt(context.Background(), owner, "image/png", "../../sensitive.png", image)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(reference, "receipt://"+owner.String()+"/"))
	require.NotContains(t, reference, "sensitive")
	restarted, err := NewLocal(directory)
	require.NoError(t, err)
	got, err := restarted.ReadReceipt(context.Background(), owner, reference)
	require.NoError(t, err)
	require.Equal(t, image, got)
	second, err := store.SaveReceipt(context.Background(), owner, "image/png", "same.png", image)
	require.NoError(t, err)
	require.NotEqual(t, reference, second)
	require.NoError(t, restarted.DeleteReceipt(context.Background(), owner, reference))
	_, err = store.ReadReceipt(context.Background(), owner, reference)
	require.True(t, errors.Is(err, os.ErrNotExist))
	require.NoError(t, store.DeleteReceipt(context.Background(), owner, reference), "cleanup is idempotent")
	entries, err := os.ReadDir(filepath.Join(directory, owner.String()))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.False(t, strings.HasSuffix(entries[0].Name(), ".part"))
}

func TestLocal_InvalidInputsAndCanceledSaveLeaveNoObjects(t *testing.T) {
	directory := t.TempDir()
	store, err := NewLocal(directory)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx   context.Context
		owner uuid.UUID
		mime  string
		bytes []byte
		want  error
	}{
		{context.Background(), uuid.Nil, "image/png", []byte{1}, apperrors.ErrInvalidInput},
		{context.Background(), uuid.New(), "text/plain", []byte{1}, apperrors.ErrInvalidInput},
		{context.Background(), uuid.New(), "image/png", nil, apperrors.ErrInvalidInput},
		{ctx, uuid.New(), "image/png", []byte{1}, context.Canceled},
	} {
		reference, err := store.SaveReceipt(tc.ctx, tc.owner, tc.mime, "receipt.png", tc.bytes)
		require.ErrorIs(t, err, tc.want)
		require.Empty(t, reference)
	}
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestLocal_ReferencesCannotEscapeStorageOrCrossOwners(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	require.NoError(t, err)
	owner := uuid.New()
	reference, err := store.SaveReceipt(context.Background(), owner, "image/jpeg", "receipt.jpg", []byte{1})
	require.NoError(t, err)
	for _, ref := range []string{"../../receipt.png", "file:///etc/passwd", "receipt://" + owner.String() + "/../../file", "receipt://" + owner.String() + "/" + uuid.NewString() + ".png?x=1", reference} {
		user := owner
		if ref == reference {
			user = uuid.New()
		}
		_, err := store.ReadReceipt(context.Background(), user, ref)
		require.ErrorIs(t, err, apperrors.ErrInvalidInput)
		require.ErrorIs(t, store.DeleteReceipt(context.Background(), user, ref), apperrors.ErrInvalidInput)
	}
}

func TestLocal_InvalidRootAndIOFailure(t *testing.T) {
	_, err := NewLocal("")
	require.Error(t, err)
	root := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(root, []byte{1}, 0600))
	_, err = NewLocal(root)
	require.Error(t, err)
	directory := t.TempDir()
	store, err := NewLocal(directory)
	require.NoError(t, err)
	owner := uuid.New()
	require.NoError(t, os.WriteFile(filepath.Join(directory, owner.String()), []byte{1}, 0600))
	_, err = store.SaveReceipt(context.Background(), owner, "image/png", "receipt.png", []byte{1})
	require.Error(t, err)
}
