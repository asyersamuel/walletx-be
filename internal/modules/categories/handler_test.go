package categories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apperrors "walletx-be/internal/shared/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// HTTP tests for the categories handler. The CategoryService is mocked with
// testify/mock; no real service, repository, or database occurs. The JWT
// middleware is replaced by a controlled fake that injects a known user_id
// string into the Gin context, matching request.ParseUserID's contract.
// nopLogger and strPtr are reused from repository_test.go (same test package).
//
// Mock strictness: positive paths register exact-argument .Once() expectations
// (including the authenticated userID, proving identity comes from the JWT
// context and never from the request body); paths that must not reach the
// service use .Maybe() + AssertNotCalled.

type mockCategoryService struct {
	mock.Mock
}

func (m *mockCategoryService) Create(ctx context.Context, userID uuid.UUID, input CreateCategoryRequest) (*Category, error) {
	args := m.Called(ctx, userID, input)
	category, _ := args.Get(0).(*Category)
	return category, args.Error(1)
}

func (m *mockCategoryService) List(ctx context.Context, userID uuid.UUID, categoryType *string) ([]Category, error) {
	args := m.Called(ctx, userID, categoryType)
	categories, _ := args.Get(0).([]Category)
	return categories, args.Error(1)
}

func (m *mockCategoryService) Update(ctx context.Context, userID, categoryID uuid.UUID, input UpdateCategoryRequest) (*Category, error) {
	args := m.Called(ctx, userID, categoryID, input)
	category, _ := args.Get(0).(*Category)
	return category, args.Error(1)
}

func (m *mockCategoryService) Delete(ctx context.Context, userID, categoryID uuid.UUID) error {
	return m.Called(ctx, userID, categoryID).Error(0)
}

var _ CategoryService = (*mockCategoryService)(nil)

// fakeAuthMiddleware simulates middleware.AuthMiddleware by injecting the
// verified identity the way request.ParseUserID expects it: a string value
// under the "user_id" key.
func fakeAuthMiddleware(userID uuid.UUID) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("user_id", userID.String())
		c.Next()
	}
}

// newAuthedRouter mirrors internal/app/router.go: categories live only under
// the JWT-protected group.
func newAuthedRouter(svc CategoryService, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	protected := r.Group("/api/v1")
	protected.Use(fakeAuthMiddleware(userID))
	RegisterProtectedRoutes(protected, NewHandler(svc, nopLogger{}))
	return r
}

// newUnauthedRouter mounts the same routes without the identity middleware to
// prove protected endpoints cannot run unauthenticated.
func newUnauthedRouter(svc CategoryService) *gin.Engine {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	RegisterProtectedRoutes(r.Group("/api/v1"), NewHandler(svc, nopLogger{}))
	return r
}

func doRequest(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, path, reader)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// categoryEnvelope decodes single-category success payloads
// (POST create / PUT update).
type categoryEnvelope struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Category struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Type      string `json:"type"`
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
		} `json:"category"`
	} `json:"data"`
}

// categoryListEnvelope decodes GET list payloads.
type categoryListEnvelope struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Categories []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Type      string `json:"type"`
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
		} `json:"categories"`
	} `json:"data"`
}

// failEnvelope decodes status/message-only error payloads.
type failEnvelope struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func decodeFailEnvelope(t *testing.T, w *httptest.ResponseRecorder) failEnvelope {
	t.Helper()
	var body failEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// categoryFixture builds a deterministic category with fixed timestamps whose
// serialized form is fixedCreatedAtString/fixedUpdatedAtString.
func categoryFixture(userID uuid.UUID, name, categoryType string) *Category {
	return &Category{
		ID:        uuid.New(),
		UserID:    userID,
		Name:      name,
		Type:      categoryType,
		CreatedAt: fixedCreatedAt,
		UpdatedAt: fixedUpdatedAt,
	}
}

func assertCategoryPayload(t *testing.T, got categoryEnvelope, expected *Category) {
	t.Helper()
	assert.Equal(t, expected.ID.String(), got.Data.Category.ID)
	assert.Equal(t, expected.Name, got.Data.Category.Name)
	assert.Equal(t, expected.Type, got.Data.Category.Type)
	assert.Equal(t, fixedCreatedAtString, got.Data.Category.CreatedAt)
	assert.Equal(t, fixedUpdatedAtString, got.Data.Category.UpdatedAt)
}

var (
	fixedCreatedAt       = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	fixedUpdatedAt       = time.Date(2026, 1, 3, 4, 5, 6, 0, time.UTC)
	fixedCreatedAtString = "2026-01-02T03:04:05Z"
	fixedUpdatedAtString = "2026-01-03T04:05:06Z"
)

func TestCategoriesHandler_Create(t *testing.T) {
	userID := uuid.New()

	t.Run("valid payload returns 201 with the created category", func(t *testing.T) {
		svc := new(mockCategoryService)
		created := categoryFixture(userID, "Food", CategoryTypeExpense)
		svc.On("Create", mock.Anything, userID, CreateCategoryRequest{Name: "Food", Type: "expense"}).
			Return(created, nil).Once()
		r := newAuthedRouter(svc, userID)

		w := doRequest(r, http.MethodPost, "/api/v1/categories", `{"name":"Food","type":"expense"}`)

		require.Equal(t, http.StatusCreated, w.Code)

		var body categoryEnvelope
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "success", body.Status)
		assert.Equal(t, "Category created successfully", body.Message)
		assertCategoryPayload(t, body, created)
		assert.NotContains(t, w.Body.String(), `"user_id"`,
			"client-facing payload must not expose ownership fields")

		svc.AssertExpectations(t)
	})

	t.Run("malformed json returns 400 without calling the service", func(t *testing.T) {
		svc := new(mockCategoryService)
		svc.On("Create", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
		r := newAuthedRouter(svc, userID)

		w := doRequest(r, http.MethodPost, "/api/v1/categories", `{"name":"Food",`)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodeFailEnvelope(t, w)
		assert.Equal(t, "fail", body.Status)
		assert.Equal(t, "Invalid request body", body.Message)
		assert.Nil(t, body.Data)

		svc.AssertNotCalled(t, "Create", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("client-supplied user_id is rejected as unknown field", func(t *testing.T) {
		svc := new(mockCategoryService)
		svc.On("Create", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
		r := newAuthedRouter(svc, userID)

		payload := fmt.Sprintf(`{"name":"Food","type":"expense","user_id":"%s"}`, uuid.NewString())
		w := doRequest(r, http.MethodPost, "/api/v1/categories", payload)

		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "Invalid request body", decodeFailEnvelope(t, w).Message)
		svc.AssertNotCalled(t, "Create", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("missing fields reach the service and map to 400", func(t *testing.T) {
		svc := new(mockCategoryService)
		invalidErr := fmt.Errorf("%w: category name must not be empty", apperrors.ErrInvalidInput)
		svc.On("Create", mock.Anything, userID, CreateCategoryRequest{}).
			Return(nil, invalidErr).Once()
		r := newAuthedRouter(svc, userID)

		w := doRequest(r, http.MethodPost, "/api/v1/categories", `{}`)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodeFailEnvelope(t, w)
		assert.Equal(t, "fail", body.Status)
		assert.Contains(t, body.Message, "category name must not be empty")

		svc.AssertExpectations(t)
	})

	t.Run("duplicate conflict returns 409", func(t *testing.T) {
		svc := new(mockCategoryService)
		svc.On("Create", mock.Anything, userID, CreateCategoryRequest{Name: "Food", Type: "expense"}).
			Return(nil, apperrors.ErrConflict).Once()
		r := newAuthedRouter(svc, userID)

		w := doRequest(r, http.MethodPost, "/api/v1/categories", `{"name":"Food","type":"expense"}`)

		require.Equal(t, http.StatusConflict, w.Code)
		body := decodeFailEnvelope(t, w)
		assert.Equal(t, "fail", body.Status)
		assert.Equal(t, "A category with this name and type already exists", body.Message)

		svc.AssertExpectations(t)
	})

	t.Run("internal service failure returns 500 without leaking details", func(t *testing.T) {
		svc := new(mockCategoryService)
		svc.On("Create", mock.Anything, userID, CreateCategoryRequest{Name: "Food", Type: "expense"}).
			Return(nil, errors.New("connection lost to primary replica XYZ")).Once()
		r := newAuthedRouter(svc, userID)

		w := doRequest(r, http.MethodPost, "/api/v1/categories", `{"name":"Food","type":"expense"}`)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodeFailEnvelope(t, w)
		assert.Equal(t, "error", body.Status)
		assert.Equal(t, "Failed to create category", body.Message)
		assert.NotContains(t, w.Body.String(), "connection lost")
		assert.NotContains(t, w.Body.String(), "XYZ")

		svc.AssertExpectations(t)
	})
}

func TestCategoriesHandler_List(t *testing.T) {
	userID := uuid.New()

	t.Run("no filter calls the service with a nil type pointer", func(t *testing.T) {
		svc := new(mockCategoryService)
		expense := categoryFixture(userID, "Food", CategoryTypeExpense)
		income := categoryFixture(userID, "Salary", CategoryTypeIncome)
		// A typed nil *string is required: an untyped nil expectation would
		// not match the *string argument via reflect.DeepEqual.
		var nilFilter *string
		svc.On("List", mock.Anything, userID, nilFilter).
			Return([]Category{*expense, *income}, nil).Once()
		r := newAuthedRouter(svc, userID)

		w := doRequest(r, http.MethodGet, "/api/v1/categories", "")

		require.Equal(t, http.StatusOK, w.Code)

		var body categoryListEnvelope
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "success", body.Status)
		assert.Equal(t, "Categories retrieved successfully", body.Message)
		require.Len(t, body.Data.Categories, 2)

		assert.Equal(t, expense.ID.String(), body.Data.Categories[0].ID)
		assert.Equal(t, "Food", body.Data.Categories[0].Name)
		assert.Equal(t, CategoryTypeExpense, body.Data.Categories[0].Type)
		assert.Equal(t, fixedCreatedAtString, body.Data.Categories[0].CreatedAt)
		assert.Equal(t, fixedUpdatedAtString, body.Data.Categories[0].UpdatedAt)
		assert.Equal(t, income.ID.String(), body.Data.Categories[1].ID)
		assert.Equal(t, "Salary", body.Data.Categories[1].Name)
		assert.Equal(t, CategoryTypeIncome, body.Data.Categories[1].Type)
		assert.NotContains(t, w.Body.String(), `"user_id"`)

		svc.AssertExpectations(t)
	})

	t.Run("valid income filter is forwarded normalized", func(t *testing.T) {
		svc := new(mockCategoryService)
		income := categoryFixture(userID, "Salary", CategoryTypeIncome)
		svc.On("List", mock.Anything, userID, strPtr(CategoryTypeIncome)).
			Return([]Category{*income}, nil).Once()
		r := newAuthedRouter(svc, userID)

		w := doRequest(r, http.MethodGet, "/api/v1/categories?type=income", "")

		require.Equal(t, http.StatusOK, w.Code)

		var body categoryListEnvelope
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body.Data.Categories, 1)
		assert.Equal(t, income.ID.String(), body.Data.Categories[0].ID)
		assert.Equal(t, CategoryTypeIncome, body.Data.Categories[0].Type)

		svc.AssertExpectations(t)
	})

	t.Run("invalid filters are rejected without calling the service", func(t *testing.T) {
		// The handler validates ?type= eagerly (case-sensitively) and never
		// reaches the service — a stricter contract than delegating to
		// service-level ErrInvalidInput, and the one pinned here.
		for _, filter := range []string{"debt", "EXPENSE", "Expenses", ""} {
			t.Run("type="+filter, func(t *testing.T) {
				svc := new(mockCategoryService)
				svc.On("List", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
				r := newAuthedRouter(svc, userID)

				w := doRequest(r, http.MethodGet, "/api/v1/categories?type="+filter, "")

				require.Equal(t, http.StatusBadRequest, w.Code)
				body := decodeFailEnvelope(t, w)
				assert.Equal(t, "fail", body.Status)
				assert.Equal(t, "invalid type filter: must be 'expense' or 'income'", body.Message)

				svc.AssertNotCalled(t, "List", mock.Anything, mock.Anything, mock.Anything)
			})
		}
	})
}

func TestCategoriesHandler_Update(t *testing.T) {
	userID := uuid.New()
	categoryID := uuid.New()
	path := "/api/v1/categories/" + categoryID.String()

	t.Run("valid payload returns 200 with the updated category", func(t *testing.T) {
		svc := new(mockCategoryService)
		updated := categoryFixture(userID, "Groceries", CategoryTypeIncome)
		updated.ID = categoryID
		svc.On("Update", mock.Anything, userID, categoryID, UpdateCategoryRequest{Name: "Groceries", Type: "income"}).
			Return(updated, nil).Once()
		r := newAuthedRouter(svc, userID)

		w := doRequest(r, http.MethodPut, path, `{"name":"Groceries","type":"income"}`)

		require.Equal(t, http.StatusOK, w.Code)

		var body categoryEnvelope
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "success", body.Status)
		assert.Equal(t, "Category updated successfully", body.Message)
		assertCategoryPayload(t, body, updated)

		svc.AssertExpectations(t)
	})

	t.Run("invalid uuid path param returns 400 without calling the service", func(t *testing.T) {
		svc := new(mockCategoryService)
		svc.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
		r := newAuthedRouter(svc, userID)

		w := doRequest(r, http.MethodPut, "/api/v1/categories/not-a-uuid", `{"name":"Groceries","type":"income"}`)

		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "Invalid category ID format", decodeFailEnvelope(t, w).Message)
		svc.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("unknown payload field returns 400 without calling the service", func(t *testing.T) {
		svc := new(mockCategoryService)
		svc.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
		r := newAuthedRouter(svc, userID)

		w := doRequest(r, http.MethodPut, path, `{"name":"Groceries","type":"income","id":"hijack"}`)

		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "Invalid request body", decodeFailEnvelope(t, w).Message)
		svc.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("service validation failure returns 400", func(t *testing.T) {
		svc := new(mockCategoryService)
		invalidErr := fmt.Errorf("%w: category name must not be empty", apperrors.ErrInvalidInput)
		svc.On("Update", mock.Anything, userID, categoryID, UpdateCategoryRequest{Name: "  ", Type: "expense"}).
			Return(nil, invalidErr).Once()
		r := newAuthedRouter(svc, userID)

		w := doRequest(r, http.MethodPut, path, `{"name":"  ","type":"expense"}`)

		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, decodeFailEnvelope(t, w).Message, "category name must not be empty")

		svc.AssertExpectations(t)
	})

	t.Run("missing or foreign category returns 404", func(t *testing.T) {
		svc := new(mockCategoryService)
		svc.On("Update", mock.Anything, userID, categoryID, UpdateCategoryRequest{Name: "Groceries", Type: "income"}).
			Return(nil, apperrors.ErrNotFound).Once()
		r := newAuthedRouter(svc, userID)

		w := doRequest(r, http.MethodPut, path, `{"name":"Groceries","type":"income"}`)

		require.Equal(t, http.StatusNotFound, w.Code)
		body := decodeFailEnvelope(t, w)
		assert.Equal(t, "fail", body.Status)
		assert.Equal(t, "Category not found", body.Message)

		svc.AssertExpectations(t)
	})

	t.Run("duplicate target combination returns 409", func(t *testing.T) {
		svc := new(mockCategoryService)
		svc.On("Update", mock.Anything, userID, categoryID, UpdateCategoryRequest{Name: "Groceries", Type: "income"}).
			Return(nil, apperrors.ErrConflict).Once()
		r := newAuthedRouter(svc, userID)

		w := doRequest(r, http.MethodPut, path, `{"name":"Groceries","type":"income"}`)

		require.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, "A category with this name and type already exists", decodeFailEnvelope(t, w).Message)

		svc.AssertExpectations(t)
	})
}

func TestCategoriesHandler_Delete(t *testing.T) {
	userID := uuid.New()
	categoryID := uuid.New()
	path := "/api/v1/categories/" + categoryID.String()

	t.Run("success returns 204 with empty body", func(t *testing.T) {
		svc := new(mockCategoryService)
		svc.On("Delete", mock.Anything, userID, categoryID).Return(nil).Once()
		r := newAuthedRouter(svc, userID)

		w := doRequest(r, http.MethodDelete, path, "")

		require.Equal(t, http.StatusNoContent, w.Code)
		assert.Empty(t, w.Body.String(), "204 responses must carry no body")

		svc.AssertExpectations(t)
	})

	t.Run("missing or foreign category returns 404", func(t *testing.T) {
		svc := new(mockCategoryService)
		svc.On("Delete", mock.Anything, userID, categoryID).Return(apperrors.ErrNotFound).Once()
		r := newAuthedRouter(svc, userID)

		w := doRequest(r, http.MethodDelete, path, "")

		require.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "Category not found", decodeFailEnvelope(t, w).Message)

		svc.AssertExpectations(t)
	})
}

func TestCategoriesHandler_Unauthenticated_Returns401WithoutCallingService(t *testing.T) {
	categoryID := uuid.New()
	svc := new(mockCategoryService)
	svc.On("Create", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	svc.On("List", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	svc.On("Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	svc.On("Delete", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	r := newUnauthedRouter(svc)

	requests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "create", method: http.MethodPost, path: "/api/v1/categories", body: `{"name":"Food","type":"expense"}`},
		{name: "list", method: http.MethodGet, path: "/api/v1/categories", body: ""},
		{name: "update", method: http.MethodPut, path: "/api/v1/categories/" + categoryID.String(), body: `{"name":"Food","type":"expense"}`},
		{name: "delete", method: http.MethodDelete, path: "/api/v1/categories/" + categoryID.String(), body: ""},
	}

	for _, rr := range requests {
		t.Run(rr.name, func(t *testing.T) {
			w := doRequest(r, rr.method, rr.path, rr.body)

			require.Equal(t, http.StatusUnauthorized, w.Code)
			body := decodeFailEnvelope(t, w)
			assert.Equal(t, "fail", body.Status)
			assert.Equal(t, "User not authenticated", body.Message)
		})
	}

	svc.AssertNotCalled(t, "Create", mock.Anything, mock.Anything, mock.Anything)
	svc.AssertNotCalled(t, "List", mock.Anything, mock.Anything, mock.Anything)
	svc.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	svc.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything)
}
