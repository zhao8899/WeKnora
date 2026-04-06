package container

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"go.uber.org/dig"
)

func TestBuildContainerProvidesCoreDependencies(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir to repo root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	t.Setenv("GIN_MODE", "release")
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "weknora-test.db"))
	t.Setenv("RETRIEVE_DRIVER", "sqlite")
	t.Setenv("STORAGE_TYPE", "dummy")
	t.Setenv("AUTO_MIGRATE", "true")
	t.Setenv("AUTO_RECOVER_DIRTY", "true")
	t.Setenv("NEO4J_ENABLE", "false")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("DOCREADER_ADDR", "")
	t.Setenv("DOCREADER_TRANSPORT", "grpc")
	t.Setenv("JWT_SECRET", "test-jwt-secret")
	t.Setenv("TENANT_AES_KEY", "weknorarag-api-key-secret-secret")
	t.Setenv("SYSTEM_AES_KEY", "weknora-system-aes-key-32bytes!!")
	t.Setenv("LOCAL_STORAGE_BASE_DIR", t.TempDir())

	c := BuildContainer(dig.New())

	var (
		cfg     *config.Config
		router  *gin.Engine
		cleaner interfaces.ResourceCleaner
	)

	if err := c.Invoke(func(gotCfg *config.Config, gotRouter *gin.Engine, gotCleaner interfaces.ResourceCleaner) {
		cfg = gotCfg
		router = gotRouter
		cleaner = gotCleaner
	}); err != nil {
		t.Fatalf("BuildContainer failed to resolve core dependencies: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected config to be resolved")
	}
	if router == nil {
		t.Fatal("expected router to be resolved")
	}
	if cleaner == nil {
		t.Fatal("expected resource cleaner to be resolved")
	}

	if errs := cleaner.Cleanup(context.Background()); len(errs) > 0 {
		t.Fatalf("expected cleanup to succeed, got %v", errs)
	}
}
