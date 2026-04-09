package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeUserRepo struct {
	usersByEmail    map[string]*types.User
	usersByUsername map[string]*types.User
	createdUser     *types.User
}

func (r *fakeUserRepo) CreateUser(ctx context.Context, user *types.User) error {
	r.createdUser = user
	if r.usersByEmail == nil {
		r.usersByEmail = map[string]*types.User{}
	}
	if r.usersByUsername == nil {
		r.usersByUsername = map[string]*types.User{}
	}
	r.usersByEmail[user.Email] = user
	r.usersByUsername[user.Username] = user
	return nil
}

func (r *fakeUserRepo) GetUserByID(ctx context.Context, id string) (*types.User, error) {
	return nil, nil
}

func (r *fakeUserRepo) GetUserByEmail(ctx context.Context, email string) (*types.User, error) {
	return r.usersByEmail[email], nil
}

func (r *fakeUserRepo) GetUserByUsername(ctx context.Context, username string) (*types.User, error) {
	return r.usersByUsername[username], nil
}

func (r *fakeUserRepo) GetUserByTenantID(ctx context.Context, tenantID uint64) (*types.User, error) {
	return nil, nil
}

func (r *fakeUserRepo) UpdateUser(ctx context.Context, user *types.User) error {
	return nil
}

func (r *fakeUserRepo) DeleteUser(ctx context.Context, id string) error {
	return nil
}

func (r *fakeUserRepo) ListUsers(ctx context.Context, offset, limit int) ([]*types.User, error) {
	return nil, nil
}

func (r *fakeUserRepo) SearchUsers(ctx context.Context, query string, limit int) ([]*types.User, error) {
	return nil, nil
}

type fakeTenantService struct {
	createdTenant *types.Tenant
	updatedTenant *types.Tenant
	tenantsByID   map[uint64]*types.Tenant
}

func (s *fakeTenantService) CreateTenant(ctx context.Context, tenant *types.Tenant) (*types.Tenant, error) {
	cloned := *tenant
	cloned.ID = 1001
	s.createdTenant = &cloned
	if s.tenantsByID == nil {
		s.tenantsByID = map[uint64]*types.Tenant{}
	}
	s.tenantsByID[cloned.ID] = &cloned
	return &cloned, nil
}

func (s *fakeTenantService) GetTenantByID(ctx context.Context, id uint64) (*types.Tenant, error) {
	tenant := s.tenantsByID[id]
	if tenant == nil {
		return nil, errors.New("record not found")
	}
	return tenant, nil
}

func (s *fakeTenantService) ListTenants(ctx context.Context) ([]*types.Tenant, error) {
	return nil, nil
}

func (s *fakeTenantService) UpdateTenant(ctx context.Context, tenant *types.Tenant) (*types.Tenant, error) {
	cloned := *tenant
	s.updatedTenant = &cloned
	return &cloned, nil
}

func (s *fakeTenantService) DeleteTenant(ctx context.Context, id uint64) error {
	return nil
}

func (s *fakeTenantService) UpdateAPIKey(ctx context.Context, id uint64) (string, error) {
	return "", nil
}

func (s *fakeTenantService) ExtractTenantIDFromAPIKey(apiKey string) (uint64, error) {
	return 0, nil
}

func (s *fakeTenantService) ListAllTenants(ctx context.Context) ([]*types.Tenant, error) {
	return nil, nil
}

func (s *fakeTenantService) SearchTenants(ctx context.Context, keyword string, tenantID uint64, page, pageSize int) ([]*types.Tenant, int64, error) {
	return nil, 0, nil
}

func (s *fakeTenantService) GetTenantByIDForUser(ctx context.Context, tenantID uint64, userID string) (*types.Tenant, error) {
	return nil, nil
}

func TestRegisterCreatesTenantAndAssignsOwnerWhenTenantIDMissing(t *testing.T) {
	userRepo := &fakeUserRepo{
		usersByEmail:    map[string]*types.User{},
		usersByUsername: map[string]*types.User{},
	}
	tenantService := &fakeTenantService{}
	svc := &userService{
		userRepo:      userRepo,
		tenantService: tenantService,
	}

	user, err := svc.Register(context.Background(), &types.RegisterRequest{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "Password123",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if tenantService.createdTenant == nil {
		t.Fatal("Register() should create a tenant when tenant_id is missing")
	}
	if tenantService.updatedTenant == nil {
		t.Fatal("Register() should update the created tenant owner")
	}
	if tenantService.updatedTenant.OwnerID != user.ID {
		t.Fatalf("Register() owner_id = %q, want %q", tenantService.updatedTenant.OwnerID, user.ID)
	}
	if user.TenantID != tenantService.createdTenant.ID {
		t.Fatalf("Register() tenant_id = %d, want %d", user.TenantID, tenantService.createdTenant.ID)
	}
}

func TestRegisterJoinsExistingTenantWithoutChangingOwner(t *testing.T) {
	userRepo := &fakeUserRepo{
		usersByEmail:    map[string]*types.User{},
		usersByUsername: map[string]*types.User{},
	}
	existingTenant := &types.Tenant{ID: 42, Name: "Existing", OwnerID: "owner-1", Status: "active"}
	tenantService := &fakeTenantService{
		tenantsByID: map[uint64]*types.Tenant{
			42: existingTenant,
		},
	}
	svc := &userService{
		userRepo:      userRepo,
		tenantService: tenantService,
	}

	user, err := svc.Register(context.Background(), &types.RegisterRequest{
		Username: "bob",
		Email:    "bob@example.com",
		Password: "Password123",
		TenantID: 42,
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if tenantService.createdTenant != nil {
		t.Fatal("Register() should not create a tenant when tenant_id is provided")
	}
	if tenantService.updatedTenant != nil {
		t.Fatal("Register() should not overwrite owner for existing tenant")
	}
	if user.TenantID != 42 {
		t.Fatalf("Register() tenant_id = %d, want 42", user.TenantID)
	}
}
