package categories

import (
	"context"
	"strings"
	"testing"
	"time"

	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Unit tests for the categories service. The repository is mocked with
// testify/mock; no database, Gin, or SQL occurs here. nopLogger and strPtr are
// reused from repository_test.go (same test package).
//
// Mock strictness: positive paths register exact-argument .Once() expectations
// verified with AssertExpectations; negative paths register .Maybe()
// expectations and assert the repository was never called.

type mockCategoryRepository struct {
	mock.Mock
}

func (m *mockCategoryRepository) Create(ctx context.Context, userID uuid.UUID, name, categoryType string) (*Category, error) {
	args := m.Called(ctx, userID, name, categoryType)
	category, _ := args.Get(0).(*Category)
	return category, args.Error(1)
}

func (m *mockCategoryRepository) ListByUser(ctx context.Context, userID uuid.UUID, categoryType *string) ([]Category, error) {
	args := m.Called(ctx, userID, categoryType)
	categories, _ := args.Get(0).([]Category)
	return categories, args.Error(1)
}

func (m *mockCategoryRepository) Update(ctx context.Context, userID, categoryID uuid.UUID, name, categoryType string) (*Category, error) {
	args := m.Called(ctx, userID, categoryID, name, categoryType)
	category, _ := args.Get(0).(*Category)
	return category, args.Error(1)
}

func (m *mockCategoryRepository) Delete(ctx context.Context, userID, categoryID uuid.UUID) error {
	return m.Called(ctx, userID, categoryID).Error(0)
}

var _ CategoryRepository = (*mockCategoryRepository)(nil)

func newTestService(repo CategoryRepository) CategoryService {
	return NewCategoryService(repo, nopLogger{})
}

func TestCategoryService_Create_NormalizesInput(t *testing.T) {
	tests := []struct {
		name         string
		inputName    string
		inputType    string
		expectedName string
		expectedType string
	}{
		{
			name:         "padded name and uppercase expense type",
			inputName:    "  Food  ",
			inputType:    "EXPENSE",
			expectedName: "Food",
			expectedType: CategoryTypeExpense,
		},
		{
			name:         "padded name and mixed-case income type",
			inputName:    " Salary ",
			inputType:    " InCoMe ",
			expectedName: "Salary",
			expectedType: CategoryTypeIncome,
		},
		{
			name:         "name of exactly the maximum length is accepted",
			inputName:    strings.Repeat("a", maxCategoryNameLength),
			inputType:    "expense",
			expectedName: strings.Repeat("a", maxCategoryNameLength),
			expectedType: CategoryTypeExpense,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			userID := uuid.New()
			repo := new(mockCategoryRepository)

			expected := &Category{
				ID:        uuid.New(),
				UserID:    userID,
				Name:      tt.expectedName,
				Type:      tt.expectedType,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			repo.On("Create", ctx, userID, tt.expectedName, tt.expectedType).
				Return(expected, nil).Once()

			svc := newTestService(repo)
			created, err := svc.Create(ctx, userID, CreateCategoryRequest{Name: tt.inputName, Type: tt.inputType})

			require.NoError(t, err)
			require.NotNil(t, created)
			assert.Equal(t, expected, created)
			assert.Equal(t, tt.expectedName, created.Name)
			assert.Equal(t, tt.expectedType, created.Type)
			assert.Equal(t, userID, created.UserID)

			repo.AssertExpectations(t)
		})
	}
}

func TestCategoryService_Create_ValidationFailures_RejectWithoutRepository(t *testing.T) {
	tests := []struct {
		name  string
		input CreateCategoryRequest
	}{
		{name: "empty name", input: CreateCategoryRequest{Name: "", Type: "expense"}},
		{name: "blank name", input: CreateCategoryRequest{Name: "   ", Type: "expense"}},
		{
			name:  "name over the maximum length",
			input: CreateCategoryRequest{Name: strings.Repeat("a", maxCategoryNameLength+1), Type: "expense"},
		},
		{name: "invalid type", input: CreateCategoryRequest{Name: "Food", Type: "investment"}},
		{name: "empty type", input: CreateCategoryRequest{Name: "Food", Type: ""}},
		{name: "blank type", input: CreateCategoryRequest{Name: "Food", Type: "   "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := new(mockCategoryRepository)
			repo.On("Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil, nil).Maybe()

			svc := newTestService(repo)
			created, err := svc.Create(ctx, uuid.New(), tt.input)

			require.Error(t, err)
			assert.ErrorIs(t, err, apperrors.ErrInvalidInput)
			assert.Nil(t, created)

			repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestCategoryService_Create_PropagatesRepositoryConflict(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	repo := new(mockCategoryRepository)
	repo.On("Create", ctx, userID, "Food", CategoryTypeExpense).
		Return(nil, apperrors.ErrConflict).Once()

	svc := newTestService(repo)
	created, err := svc.Create(ctx, userID, CreateCategoryRequest{Name: "Food", Type: "expense"})

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrConflict,
		"duplicate (user_id, name, type) must keep its conflict classification for 409 mapping")
	assert.Nil(t, created)

	repo.AssertExpectations(t)
}

func TestCategoryService_List_NilFilter_PassesNilToRepository(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	repo := new(mockCategoryRepository)

	expected := []Category{
		{ID: uuid.New(), UserID: userID, Name: "Food", Type: CategoryTypeExpense, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: uuid.New(), UserID: userID, Name: "Salary", Type: CategoryTypeIncome, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	// A typed nil *string is required here: an untyped nil expectation would
	// not match the *string argument via reflect.DeepEqual.
	var nilFilter *string
	repo.On("ListByUser", ctx, userID, nilFilter).Return(expected, nil).Once()

	svc := newTestService(repo)
	list, err := svc.List(ctx, userID, nil)

	require.NoError(t, err)
	assert.Equal(t, expected, list)

	repo.AssertExpectations(t)
}

func TestCategoryService_List_ValidFilter_NormalizesBeforeRepositoryCall(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	repo := new(mockCategoryRepository)

	expected := []Category{
		{ID: uuid.New(), UserID: userID, Name: "Salary", Type: CategoryTypeIncome, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	repo.On("ListByUser", ctx, userID, strPtr(CategoryTypeIncome)).Return(expected, nil).Once()

	svc := newTestService(repo)
	list, err := svc.List(ctx, userID, strPtr(" InCoMe "))

	require.NoError(t, err)
	assert.Equal(t, expected, list)

	repo.AssertExpectations(t)
}

func TestCategoryService_List_InvalidFilter_RejectsWithoutRepository(t *testing.T) {
	for _, filter := range []string{"debt", "Expenses", ""} {
		t.Run("filter "+filter, func(t *testing.T) {
			ctx := context.Background()
			repo := new(mockCategoryRepository)
			repo.On("ListByUser", mock.Anything, mock.Anything, mock.Anything).
				Return(nil, nil).Maybe()

			svc := newTestService(repo)
			list, err := svc.List(ctx, uuid.New(), strPtr(filter))

			require.Error(t, err)
			assert.ErrorIs(t, err, apperrors.ErrInvalidInput)
			assert.Nil(t, list)

			repo.AssertNotCalled(t, "ListByUser", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestCategoryService_Update_NormalizesBothInputsAndDelegates(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	categoryID := uuid.New()
	repo := new(mockCategoryRepository)

	updated := &Category{
		ID:        categoryID,
		UserID:    userID,
		Name:      "Groceries",
		Type:      CategoryTypeIncome,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	repo.On("Update", ctx, userID, categoryID, "Groceries", CategoryTypeIncome).
		Return(updated, nil).Once()

	svc := newTestService(repo)
	result, err := svc.Update(ctx, userID, categoryID, UpdateCategoryRequest{Name: "  Groceries  ", Type: " INCOME "})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, updated, result)
	assert.Equal(t, categoryID, result.ID)
	assert.Equal(t, userID, result.UserID)

	repo.AssertExpectations(t)
}

func TestCategoryService_Update_PropagatesRepositoryErrors(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	categoryID := uuid.New()
	input := UpdateCategoryRequest{Name: "Food", Type: "expense"}

	t.Run("conflict on duplicate target combination", func(t *testing.T) {
		repo := new(mockCategoryRepository)
		repo.On("Update", ctx, userID, categoryID, "Food", CategoryTypeExpense).
			Return(nil, apperrors.ErrConflict).Once()

		svc := newTestService(repo)
		result, err := svc.Update(ctx, userID, categoryID, input)

		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrConflict,
			"duplicate (user_id, name, type) target must keep its conflict classification")
		assert.Nil(t, result)

		repo.AssertExpectations(t)
	})

	t.Run("not found for missing or foreign category", func(t *testing.T) {
		repo := new(mockCategoryRepository)
		repo.On("Update", ctx, userID, categoryID, "Food", CategoryTypeExpense).
			Return(nil, apperrors.ErrNotFound).Once()

		svc := newTestService(repo)
		result, err := svc.Update(ctx, userID, categoryID, input)

		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrNotFound)
		assert.Nil(t, result)

		repo.AssertExpectations(t)
	})
}

func TestCategoryService_Update_ValidationFailures_RejectWithoutRepository(t *testing.T) {
	tests := []struct {
		name  string
		input UpdateCategoryRequest
	}{
		{name: "blank name", input: UpdateCategoryRequest{Name: "   ", Type: "expense"}},
		{
			name:  "name over the maximum length",
			input: UpdateCategoryRequest{Name: strings.Repeat("a", maxCategoryNameLength+1), Type: "income"},
		},
		{name: "invalid type", input: UpdateCategoryRequest{Name: "Food", Type: "investment"}},
		{name: "empty type", input: UpdateCategoryRequest{Name: "Food", Type: ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := new(mockCategoryRepository)
			repo.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil, nil).Maybe()

			svc := newTestService(repo)
			result, err := svc.Update(ctx, uuid.New(), uuid.New(), tt.input)

			require.Error(t, err)
			assert.ErrorIs(t, err, apperrors.ErrInvalidInput)
			assert.Nil(t, result)

			repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestCategoryService_Delete_DelegatesWithOwnershipIdentity(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	categoryID := uuid.New()
	repo := new(mockCategoryRepository)
	repo.On("Delete", ctx, userID, categoryID).Return(nil).Once()

	svc := newTestService(repo)
	err := svc.Delete(ctx, userID, categoryID)

	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestCategoryService_Delete_PropagatesNotFound(t *testing.T) {
	ctx := context.Background()
	repo := new(mockCategoryRepository)
	repo.On("Delete", ctx, mock.Anything, mock.Anything).Return(apperrors.ErrNotFound).Once()

	svc := newTestService(repo)
	err := svc.Delete(ctx, uuid.New(), uuid.New())

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)

	repo.AssertExpectations(t)
}
