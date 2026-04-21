# Plan C: Connector Framework — Pre-Work Fixes + Web/RSS Connectors

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复现有 datasource 框架的三个实现偏差（archived 语义、外部 ID 索引、Config 加密），然后实现 Web/RSS 连接器，打通"抓取 → 摘要 → 入库 → 变更检测 → archived 归档"完整链路。

**Architecture:** 在现有 `internal/datasource/` 框架上叠加——不重写 Connector 接口和 Scheduler（已完善），只修复具体错误行为并新增两个 Connector 实现。Web 连接器使用已有的 `goquery` + `html-to-markdown`；RSS 连接器新增 `gofeed` 依赖。

**Tech Stack:** Go, GORM, PostgreSQL, goquery（已有）, html-to-markdown（已有）, gofeed（新增）

**当前 datasource 框架状态（勿重复实现）：**
- `Connector` 接口：已定义（`internal/datasource/connector.go`），有 Type/Validate/ListResources/FetchAll/FetchIncremental
- `ConnectorRegistry` + `Scheduler`：已实现
- `data_sources` + `sync_logs` 表：已存在（migration 000028/000029）
- Feishu connector：已实现，参考路径 `internal/datasource/connector/feishu/`
- Web Crawler、RSS：已在 `ConnectorMetadataRegistry` 注册，但无实现文件

**已知的三个实现偏差：**
1. `IsDeleted` 时（`datasource_service.go:514`）仅计数，未改 knowledge.status → 已删除文档仍可被检索
2. `external_id` 匹配后做 delete+recreate（`datasource_service.go:651-665`）→ ID 变化，answer_evidence 引用断裂
3. `data_sources.config` 明文 JSONB → 凭证泄露风险

---

## 文件清单

新建：
- `migrations/versioned/000035_knowledge_external_id.up.sql`
- `migrations/versioned/000035_knowledge_external_id.down.sql`
- `migrations/versioned/000036_datasource_config_encrypted.up.sql`
- `migrations/versioned/000036_datasource_config_encrypted.down.sql`
- `internal/crypto/config_crypt.go`
- `internal/crypto/config_crypt_test.go`
- `internal/datasource/connector/web/connector.go`
- `internal/datasource/connector/web/connector_test.go`
- `internal/datasource/connector/rss/connector.go`
- `internal/datasource/connector/rss/connector_test.go`

修改：
- `internal/types/knowledge.go` — 新增 `ManualKnowledgeStatusArchived` 常量
- `internal/application/service/datasource_service.go` — 修复 archived 语义（Task 1）
- `internal/types/datasource.go` — `ParseConfig` 支持解密，新增 `SaveConfig` 加密写入（Task 3）
- `internal/container/container.go` — 注册 Web/RSS connector（Task 6）
- `go.mod` + `go.sum` — 新增 `gofeed` 依赖（Task 5）

---

## Task 1: Pre-work A — Archived 语义修复

修复：当 `item.IsDeleted == true && ds.SyncDeletions == true` 时，把对应 knowledge 条目状态改为 `archived`，而不是只计数。

**Files:**
- Modify: `internal/types/knowledge.go`
- Modify: `internal/application/service/datasource_service.go`
- Modify: `internal/types/interfaces/` 下的 knowledge repository 接口文件（添加 `UpdateStatus`）
- Modify: knowledge repository 实现文件（添加 `UpdateStatus` 实现）

- [ ] **Step 1: 定位 knowledge repository 接口和实现**

```bash
grep -rn "FindByMetadataKey\|KnowledgeRepository" internal/types/interfaces/ | head -10
grep -rn "func.*FindByMetadataKey" internal/application/repository/ | head -5
```

记录接口文件路径和实现文件路径，后续步骤需要修改这两个文件。

- [ ] **Step 2: 在 knowledge.go 中添加 Archived 状态常量**

找到文件中 `ManualKnowledgeStatusDraft` / `ManualKnowledgeStatusPublish` 所在的 const 块，追加：

```go
ManualKnowledgeStatusArchived = "archived"
```

- [ ] **Step 3: 在 KnowledgeRepository 接口中添加 UpdateStatus**

在 Step 1 找到的接口文件中，在 `FindByMetadataKey` 方法附近添加：

```go
UpdateStatus(ctx context.Context, id string, status string) error
```

- [ ] **Step 4: 在 repository 实现中添加 UpdateStatus**

在 Step 1 找到的实现文件中，添加：

```go
func (r *KnowledgeRepository) UpdateStatus(ctx context.Context, id string, status string) error {
    return r.db.WithContext(ctx).
        Model(&types.KnowledgeDetails{}).
        Where("id = ?", id).
        Update("parse_status", status).Error
}
```

注意：如果 knowledge 的状态字段不叫 `parse_status`，运行 `grep -n "parse_status\|ParseStatus" internal/types/knowledge.go | head -5` 确认实际字段名，再替换。

- [ ] **Step 5: 修复 datasource_service.go 中的 IsDeleted 处理**

找到 `datasource_service.go` 约第 513-518 行的：

```go
if item.IsDeleted {
    if ds.SyncDeletions {
        result.Deleted++
    }
    continue
}
```

替换为：

```go
if item.IsDeleted {
    if ds.SyncDeletions && item.ExternalID != "" {
        repo := s.knowledgeService.GetRepository()
        existing, err := repo.FindByMetadataKey(ctx, ds.TenantID, ds.KnowledgeBaseID, "external_id", item.ExternalID)
        if err != nil {
            logger.Warnf(ctx, "archive: failed to find knowledge for external_id=%s: %v", item.ExternalID, err)
        } else if existing != nil && existing.ParseStatus != types.ManualKnowledgeStatusArchived {
            if err := repo.UpdateStatus(ctx, existing.ID, types.ManualKnowledgeStatusArchived); err != nil {
                logger.Warnf(ctx, "archive: failed to set archived for knowledge=%s: %v", existing.ID, err)
            } else {
                logger.Infof(ctx, "archived knowledge %s (external_id=%s)", existing.ID, item.ExternalID)
                result.Deleted++
            }
        }
    }
    continue
}
```

- [ ] **Step 6: 确认编译**

```bash
make build 2>&1 | head -20
```

Expected: build 成功。

- [ ] **Step 7: Commit**

```bash
git add internal/types/knowledge.go \
         internal/application/service/datasource_service.go \
         internal/types/interfaces/ \
         internal/application/repository/
git commit -m "fix(datasource): archive knowledge on remote deletion instead of silently skipping"
```

---

## Task 2: Pre-work B — external_id 列索引

当前 `external_id` 存在 `knowledge.metadata JSONB` 中，查找走全表扫描。新增专用列以加速查找。

**注：完整的"原地更新（不改 ID）"需要 knowledge service 的 UpdateContent+Revectorize 路径（涉及 chunking+embedding pipeline），不在本 Plan 范围内。本 Task 仅添加列和索引，以及 `FindByExternalID` 快速查找方法。**

**Files:**
- Create: `migrations/versioned/000035_knowledge_external_id.up.sql`
- Create: `migrations/versioned/000035_knowledge_external_id.down.sql`

- [ ] **Step 1: 写入 up migration**

```sql
-- migrations/versioned/000035_knowledge_external_id.up.sql
DO $$ BEGIN RAISE NOTICE '[Migration 000035] Adding external_id column to knowledge'; END $$;

ALTER TABLE knowledge ADD COLUMN IF NOT EXISTS external_id TEXT;
CREATE INDEX IF NOT EXISTS idx_knowledge_external_id ON knowledge(external_id) WHERE external_id IS NOT NULL;

-- Backfill from existing metadata JSONB
UPDATE knowledge
SET external_id = metadata->>'external_id'
WHERE metadata->>'external_id' IS NOT NULL AND external_id IS NULL;

COMMENT ON COLUMN knowledge.external_id IS '第三方数据源的原始文档 ID，用于变更检测和去重';

DO $$ BEGIN RAISE NOTICE '[Migration 000035] external_id column added and backfilled'; END $$;
```

- [ ] **Step 2: 写入 down migration**

```sql
-- migrations/versioned/000035_knowledge_external_id.down.sql
DROP INDEX IF EXISTS idx_knowledge_external_id;
ALTER TABLE knowledge DROP COLUMN IF EXISTS external_id;
```

- [ ] **Step 3: 执行迁移**

```bash
make migrate-up
```

Expected: migration 000035 applied without error.

- [ ] **Step 4: 验证列存在**

```bash
psql $DATABASE_URL -c "\d knowledge" | grep external_id
```

Expected: `external_id | text | ...`

- [ ] **Step 5: 在 KnowledgeRepository 接口和实现中添加 FindByExternalID**

接口：

```go
FindByExternalID(ctx context.Context, tenantID uint64, kbID, externalID string) (*types.KnowledgeDetails, error)
```

实现（在 repository 实现文件中）：

```go
func (r *KnowledgeRepository) FindByExternalID(ctx context.Context, tenantID uint64, kbID, externalID string) (*types.KnowledgeDetails, error) {
    var k types.KnowledgeDetails
    err := r.db.WithContext(ctx).
        Where("tenant_id = ? AND knowledge_base_id = ? AND external_id = ? AND deleted_at IS NULL", tenantID, kbID, externalID).
        First(&k).Error
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return nil, nil
    }
    return &k, err
}
```

- [ ] **Step 6: 在 ingestItem 中优先使用 FindByExternalID**

在 `datasource_service.go` 的 `ingestItem` 函数中，找到 `repo.FindByMetadataKey(ctx, ..., "external_id", item.ExternalID)` 调用，替换为：

```go
existing, err := repo.FindByExternalID(ctx, ds.TenantID, ds.KnowledgeBaseID, item.ExternalID)
```

同时在 `ingestItem` 的 `metadata` map 中确保写入 `external_id` 字段（已有），并在创建知识条目时同时填充 `external_id` 列。查找具体的 `CreateKnowledgeFromFile` / `CreateKnowledgeFromURL` 调用，确认 `metadata["external_id"]` 会被同步到 `knowledge.external_id` 列（如果 knowledge service 不自动同步，则在创建后手动 `UPDATE knowledge SET external_id = ? WHERE id = ?`）。

- [ ] **Step 7: 确认编译**

```bash
make build 2>&1 | head -20
```

- [ ] **Step 8: Commit**

```bash
git add migrations/versioned/000035_knowledge_external_id.up.sql \
         migrations/versioned/000035_knowledge_external_id.down.sql \
         internal/types/interfaces/ \
         internal/application/repository/ \
         internal/application/service/datasource_service.go
git commit -m "feat(migration): add knowledge.external_id index for fast connector lookup (000035)"
```

---

## Task 3: Pre-work C — Config AES-256 加密

凭证（API Key、Token）以明文 JSONB 存入数据库。新增 `config_encrypted TEXT` 列：写入时加密，读取时优先解密，旧明文记录向后兼容。

密钥通过环境变量 `DATA_SOURCE_CONFIG_KEY`（64 位十六进制 = 32 字节 AES-256 密钥）注入。若未设置，写入时跳过加密，读取时走明文路径。

**Files:**
- Create: `migrations/versioned/000036_datasource_config_encrypted.up.sql`
- Create: `migrations/versioned/000036_datasource_config_encrypted.down.sql`
- Create: `internal/crypto/config_crypt.go`
- Create: `internal/crypto/config_crypt_test.go`
- Modify: `internal/types/datasource.go`

- [ ] **Step 1: 写入 migration**

```sql
-- migrations/versioned/000036_datasource_config_encrypted.up.sql
DO $$ BEGIN RAISE NOTICE '[Migration 000036] Adding config_encrypted column to data_sources'; END $$;

ALTER TABLE data_sources ADD COLUMN IF NOT EXISTS config_encrypted TEXT;
COMMENT ON COLUMN data_sources.config_encrypted IS 'AES-256-GCM encrypted JSON config (base64 nonce+ciphertext). Supersedes config JSONB when present.';

DO $$ BEGIN RAISE NOTICE '[Migration 000036] config_encrypted column added'; END $$;
```

```sql
-- migrations/versioned/000036_datasource_config_encrypted.down.sql
ALTER TABLE data_sources DROP COLUMN IF EXISTS config_encrypted;
```

- [ ] **Step 2: 执行迁移**

```bash
make migrate-up
```

- [ ] **Step 3: 写入 crypto 包**

```go
// internal/crypto/config_crypt.go
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
)

// Encrypt encrypts plaintext with AES-256-GCM using a 64-char hex key.
// Returns base64-encoded ciphertext with nonce prepended.
func Encrypt(plaintext []byte, keyHex string) (string, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return "", errors.New("config_crypt: key must be 64 hex chars (32 bytes)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt decrypts a base64-encoded AES-256-GCM ciphertext.
func Decrypt(ciphertext string, keyHex string) ([]byte, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return nil, errors.New("config_crypt: key must be 64 hex chars (32 bytes)")
	}
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("config_crypt: ciphertext too short")
	}
	nonce, ciphertextBytes := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertextBytes, nil)
}
```

- [ ] **Step 4: 写 crypto 包单元测试**

```go
// internal/crypto/config_crypt_test.go
package crypto_test

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/crypto"
)

const testKey = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

func TestEncryptDecryptRoundtrip(t *testing.T) {
	plaintext := []byte(`{"credentials":{"api_key":"secret-value"}}`)
	ciphertext, err := crypto.Encrypt(plaintext, testKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if ciphertext == string(plaintext) {
		t.Fatal("ciphertext should differ from plaintext")
	}
	decrypted, err := crypto.Decrypt(ciphertext, testKey)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptProducesDistinctCiphertexts(t *testing.T) {
	pt := []byte("same plaintext")
	c1, _ := crypto.Encrypt(pt, testKey)
	c2, _ := crypto.Encrypt(pt, testKey)
	if c1 == c2 {
		t.Fatal("two encryptions of same plaintext should differ (random nonce)")
	}
}

func TestDecryptInvalidKeyReturnsError(t *testing.T) {
	_, err := crypto.Decrypt("anyciphertext", "tooshort")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}
```

- [ ] **Step 5: 运行 crypto 测试**

```bash
go test ./internal/crypto/... -v
```

Expected: PASS（3 tests）.

- [ ] **Step 6: 在 DataSource 上添加 ConfigEncrypted 字段和加密读写方法**

在 `internal/types/datasource.go` 中找到 `DataSource` struct，添加字段（与 `Config JSON` 相邻）：

```go
ConfigEncrypted string `json:"config_encrypted,omitempty" gorm:"column:config_encrypted"`
```

找到现有 `ParseConfig()` 方法，在方法开头插入解密优先逻辑：

```go
func (d *DataSource) ParseConfig() (*DataSourceConfig, error) {
	// Prefer encrypted config when present and encryption key is configured
	if d.ConfigEncrypted != "" {
		if keyHex := os.Getenv("DATA_SOURCE_CONFIG_KEY"); keyHex != "" {
			plaintext, err := configcrypto.Decrypt(d.ConfigEncrypted, keyHex)
			if err != nil {
				return nil, fmt.Errorf("decrypt config: %w", err)
			}
			var cfg DataSourceConfig
			return &cfg, json.Unmarshal(plaintext, &cfg)
		}
	}
	// Fallback: parse plaintext JSONB (backward compatible with existing records)
	var cfg DataSourceConfig
	if len(d.Config) == 0 {
		return &cfg, nil
	}
	return &cfg, json.Unmarshal(d.Config, &cfg)
}
```

Add new `SaveConfig()` method right after `ParseConfig()`:

```go
// SaveConfig serializes cfg and encrypts it to ConfigEncrypted when DATA_SOURCE_CONFIG_KEY is set.
// Falls back to plaintext Config JSONB when key is not configured.
func (d *DataSource) SaveConfig(cfg *DataSourceConfig) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if keyHex := os.Getenv("DATA_SOURCE_CONFIG_KEY"); keyHex != "" {
		encrypted, err := configcrypto.Encrypt(b, keyHex)
		if err != nil {
			return fmt.Errorf("encrypt config: %w", err)
		}
		d.ConfigEncrypted = encrypted
		return nil
	}
	d.Config = b
	return nil
}
```

Add imports to `internal/types/datasource.go`:
```go
import (
    // ... existing imports ...
    "os"
    configcrypto "github.com/Tencent/WeKnora/internal/crypto"
)
```

- [ ] **Step 7: 确认编译**

```bash
make build 2>&1 | head -20
```

Expected: build 成功。

- [ ] **Step 8: Commit**

```bash
git add migrations/versioned/000036_datasource_config_encrypted.up.sql \
         migrations/versioned/000036_datasource_config_encrypted.down.sql \
         internal/crypto/ \
         internal/types/datasource.go
git commit -m "feat(security): add AES-256-GCM config encryption for data_sources (migration 000036)"
```

---

## Task 4: Web Crawler Connector

实现 `datasource.Connector` 接口的 Web 抓取版本：单页 URL 抓取 / 站点地图展开 / ETag+内容 hash 变更检测。

**Files:**
- Create: `internal/datasource/connector/web/connector.go`
- Create: `internal/datasource/connector/web/connector_test.go`

依赖（已在 go.mod 中）：
- `github.com/PuerkitoBio/goquery`
- `github.com/JohannesKaufmann/html-to-markdown/v2`

配置结构（填入 `DataSourceConfig.Settings`）：
```json
{
  "urls": ["https://example.com/docs/"],
  "sitemap_url": "https://example.com/sitemap.xml",
  "user_agent": "WeKnora-Crawler/1.0"
}
```

- [ ] **Step 1: 写入 connector.go**

```go
// internal/datasource/connector/web/connector.go
package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/PuerkitoBio/goquery"
	"github.com/Tencent/WeKnora/internal/types"
)

const connectorType = "web_crawler"

// Connector implements datasource.Connector for web page and sitemap crawling.
type Connector struct {
	client *http.Client
}

// New creates a new web Connector.
func New() *Connector {
	return &Connector{client: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Connector) Type() string { return connectorType }

// Validate checks that the configured URL or sitemap is reachable.
func (c *Connector) Validate(ctx context.Context, config *types.DataSourceConfig) error {
	urls := configURLs(config)
	sm := sitemapURL(config)
	if len(urls) == 0 && sm == "" {
		return fmt.Errorf("web_crawler: at least one URL or sitemap_url is required in settings")
	}
	checkURL := sm
	if checkURL == "" {
		checkURL = urls[0]
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, checkURL, nil)
	if err != nil {
		return fmt.Errorf("web_crawler: invalid URL %q: %w", checkURL, err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("web_crawler: cannot reach %q: %w", checkURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("web_crawler: %q returned HTTP %d", checkURL, resp.StatusCode)
	}
	return nil
}

// ListResources expands sitemap URLs or returns the configured URL list.
func (c *Connector) ListResources(ctx context.Context, config *types.DataSourceConfig) ([]types.Resource, error) {
	if sm := sitemapURL(config); sm != "" {
		urls, err := c.parseSitemap(ctx, sm)
		if err != nil {
			return nil, fmt.Errorf("web_crawler: parse sitemap: %w", err)
		}
		resources := make([]types.Resource, 0, len(urls))
		for _, u := range urls {
			resources = append(resources, types.Resource{ExternalID: urlHash(u), Name: u, Type: "url", URL: u})
		}
		return resources, nil
	}
	urls := configURLs(config)
	resources := make([]types.Resource, 0, len(urls))
	for _, u := range urls {
		resources = append(resources, types.Resource{ExternalID: urlHash(u), Name: u, Type: "url", URL: u})
	}
	return resources, nil
}

// FetchAll fetches all configured URLs as Markdown documents.
func (c *Connector) FetchAll(ctx context.Context, config *types.DataSourceConfig, resourceIDs []string) ([]types.FetchedItem, error) {
	resources, err := c.ListResources(ctx, config)
	if err != nil {
		return nil, err
	}
	idSet := make(map[string]bool, len(resourceIDs))
	for _, id := range resourceIDs {
		idSet[id] = true
	}
	ua := userAgent(config)
	var items []types.FetchedItem
	for _, r := range resources {
		if len(idSet) > 0 && !idSet[r.ExternalID] {
			continue
		}
		item, err := c.fetchPage(ctx, r.URL, ua)
		if err != nil {
			items = append(items, types.FetchedItem{
				ExternalID: r.ExternalID,
				Title:      r.URL,
				Metadata:   map[string]string{"error": err.Error()},
			})
			continue
		}
		item.ExternalID = r.ExternalID
		items = append(items, *item)
	}
	return items, nil
}

// FetchIncremental returns only pages that have changed since the last cursor
// using ETag / Last-Modified headers or content hash as fallback.
func (c *Connector) FetchIncremental(ctx context.Context, config *types.DataSourceConfig, cursor *types.SyncCursor) ([]types.FetchedItem, *types.SyncCursor, error) {
	resources, err := c.ListResources(ctx, config)
	if err != nil {
		return nil, nil, err
	}
	ua := userAgent(config)

	prevState := map[string]string{}
	if cursor != nil {
		if v, ok := cursor.ConnectorCursor["page_etags"]; ok {
			if m, ok := v.(map[string]interface{}); ok {
				for k, val := range m {
					if s, ok := val.(string); ok {
						prevState[k] = s
					}
				}
			}
		}
	}

	newState := make(map[string]interface{}, len(resources))
	var changed []types.FetchedItem

	for _, r := range resources {
		etag, lastMod, _ := c.headURL(ctx, r.URL, ua)
		headerKey := etag + "|" + lastMod
		if headerKey != "|" && prevState[r.ExternalID] == headerKey {
			newState[r.ExternalID] = headerKey
			continue
		}
		item, err := c.fetchPage(ctx, r.URL, ua)
		if err != nil {
			continue
		}
		item.ExternalID = r.ExternalID
		contentHash := sha256hex(item.Content)
		if prevState[r.ExternalID] == contentHash {
			newState[r.ExternalID] = contentHash
			continue
		}
		changed = append(changed, *item)
		if headerKey != "|" {
			newState[r.ExternalID] = headerKey
		} else {
			newState[r.ExternalID] = contentHash
		}
	}

	nextCursor := &types.SyncCursor{
		LastSyncTime:    time.Now(),
		ConnectorCursor: map[string]interface{}{"page_etags": newState},
	}
	return changed, nextCursor, nil
}

// --- private helpers ---

func (c *Connector) fetchPage(ctx context.Context, pageURL, ua string) (*types.FetchedItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(doc.Find("title").First().Text())
	if title == "" {
		title = pageURL
	}
	doc.Find("nav,header,footer,script,style,[aria-hidden='true']").Remove()
	htmlContent, _ := doc.Find("body").Html()
	converter := md.NewConverter("", true, nil)
	markdown, err := converter.ConvertString(htmlContent)
	if err != nil {
		markdown = doc.Text()
	}
	return &types.FetchedItem{
		Title:       title,
		Content:     []byte(markdown),
		ContentType: "text/markdown",
		FileName:    safeFilename(title) + ".md",
		URL:         pageURL,
		UpdatedAt:   time.Now(),
		Metadata:    map[string]string{"source_url": pageURL},
	}, nil
}

func (c *Connector) headURL(ctx context.Context, pageURL, ua string) (etag, lastMod string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, pageURL, nil)
	if err != nil {
		return "", "", err
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	return resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), nil
}

type sitemapEntry struct {
	Loc string `xml:"loc"`
}

type sitemapDoc struct {
	XMLName xml.Name       `xml:"urlset"`
	URLs    []sitemapEntry `xml:"url"`
}

func (c *Connector) parseSitemap(ctx context.Context, sitemapAddr string) ([]string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, sitemapAddr, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var doc sitemapDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	urls := make([]string, 0, len(doc.URLs))
	for _, u := range doc.URLs {
		if u.Loc != "" {
			urls = append(urls, u.Loc)
		}
	}
	return urls, nil
}

func configURLs(config *types.DataSourceConfig) []string {
	if config == nil || config.Settings == nil {
		return nil
	}
	raw, ok := config.Settings["urls"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

func sitemapURL(config *types.DataSourceConfig) string {
	if config == nil || config.Settings == nil {
		return ""
	}
	v, _ := config.Settings["sitemap_url"].(string)
	return v
}

func userAgent(config *types.DataSourceConfig) string {
	if config == nil || config.Settings == nil {
		return "WeKnora-Crawler/1.0"
	}
	if v, ok := config.Settings["user_agent"].(string); ok && v != "" {
		return v
	}
	return "WeKnora-Crawler/1.0"
}

func urlHash(u string) string {
	h := sha256.Sum256([]byte(u))
	return hex.EncodeToString(h[:8])
}

func sha256hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func safeFilename(s string) string {
	r := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "-", "?", "-", `"`, "-", "<", "-", ">", "-", "|", "-")
	result := r.Replace(s)
	if len(result) > 100 {
		result = result[:100]
	}
	return result
}
```

- [ ] **Step 2: 写入 connector_test.go**

```go
// internal/datasource/connector/web/connector_test.go
package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/datasource/connector/web"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestValidateReachableURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := web.New()
	config := &types.DataSourceConfig{
		Settings: map[string]interface{}{"urls": []interface{}{srv.URL}},
	}
	if err := c.Validate(context.Background(), config); err != nil {
		t.Fatalf("Validate returned unexpected error: %v", err)
	}
}

func TestValidateNoURLReturnsError(t *testing.T) {
	c := web.New()
	config := &types.DataSourceConfig{Settings: map[string]interface{}{}}
	if err := c.Validate(context.Background(), config); err == nil {
		t.Fatal("expected error when no URL configured")
	}
}

func TestFetchAllConvertsHTMLToMarkdown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>Test Page</title></head><body><h1>Hello</h1><p>World</p></body></html>`))
	}))
	defer srv.Close()

	c := web.New()
	config := &types.DataSourceConfig{
		Settings: map[string]interface{}{"urls": []interface{}{srv.URL}},
	}
	items, err := c.FetchAll(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("FetchAll error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Title != "Test Page" {
		t.Errorf("title = %q, want %q", items[0].Title, "Test Page")
	}
	if len(items[0].Content) == 0 {
		t.Error("content should not be empty after HTML→Markdown conversion")
	}
}
```

- [ ] **Step 3: 确认编译和测试**

```bash
make build 2>&1 | head -20
go test ./internal/datasource/connector/web/... -v
```

Expected: build 成功，tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/datasource/connector/web/
git commit -m "feat(connector): add Web Crawler connector with sitemap + ETag change detection"
```

---

## Task 5: RSS Feed Connector

**Files:**
- Create: `internal/datasource/connector/rss/connector.go`
- Create: `internal/datasource/connector/rss/connector_test.go`

新增依赖：`github.com/mmcdole/gofeed`（Apache 2.0）

配置结构（填入 `DataSourceConfig.Settings`）：
```json
{
  "feed_url": "https://example.com/feed.xml",
  "max_items": 100
}
```

- [ ] **Step 1: 添加 gofeed 依赖**

```bash
go get github.com/mmcdole/gofeed@latest
```

Expected: go.mod 和 go.sum 更新。

- [ ] **Step 2: 写入 connector.go**

```go
// internal/datasource/connector/rss/connector.go
package rss

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/mmcdole/gofeed"
)

const connectorType = "rss"

// Connector implements datasource.Connector for RSS/Atom feeds.
type Connector struct {
	parser *gofeed.Parser
}

// New creates a new RSS Connector.
func New() *Connector {
	return &Connector{parser: gofeed.NewParser()}
}

func (c *Connector) Type() string { return connectorType }

// Validate checks that the feed URL is parseable and returns at least one item.
func (c *Connector) Validate(ctx context.Context, config *types.DataSourceConfig) error {
	url := feedURL(config)
	if url == "" {
		return fmt.Errorf("rss: feed_url is required in settings")
	}
	feed, err := c.parser.ParseURLWithContext(url, ctx)
	if err != nil {
		return fmt.Errorf("rss: cannot parse feed %q: %w", url, err)
	}
	if len(feed.Items) == 0 {
		return fmt.Errorf("rss: feed %q returned 0 items", url)
	}
	return nil
}

// ListResources returns feed items as resources.
func (c *Connector) ListResources(ctx context.Context, config *types.DataSourceConfig) ([]types.Resource, error) {
	feed, err := c.parser.ParseURLWithContext(feedURL(config), ctx)
	if err != nil {
		return nil, fmt.Errorf("rss: parse feed: %w", err)
	}
	limit := maxItems(config)
	resources := make([]types.Resource, 0, len(feed.Items))
	for i, item := range feed.Items {
		if limit > 0 && i >= limit {
			break
		}
		modAt := itemTime(item)
		resources = append(resources, types.Resource{
			ExternalID: itemID(item),
			Name:       item.Title,
			Type:       "article",
			URL:        item.Link,
			ModifiedAt: modAt,
		})
	}
	return resources, nil
}

// FetchAll fetches all current feed items up to max_items.
func (c *Connector) FetchAll(ctx context.Context, config *types.DataSourceConfig, resourceIDs []string) ([]types.FetchedItem, error) {
	feed, err := c.parser.ParseURLWithContext(feedURL(config), ctx)
	if err != nil {
		return nil, fmt.Errorf("rss: parse feed: %w", err)
	}
	idSet := make(map[string]bool, len(resourceIDs))
	for _, id := range resourceIDs {
		idSet[id] = true
	}
	limit := maxItems(config)
	var items []types.FetchedItem
	for i, fi := range feed.Items {
		if limit > 0 && i >= limit {
			break
		}
		if len(idSet) > 0 && !idSet[itemID(fi)] {
			continue
		}
		items = append(items, feedItemToFetchedItem(fi))
	}
	return items, nil
}

// FetchIncremental returns only items published/updated after cursor.LastSyncTime.
func (c *Connector) FetchIncremental(ctx context.Context, config *types.DataSourceConfig, cursor *types.SyncCursor) ([]types.FetchedItem, *types.SyncCursor, error) {
	feed, err := c.parser.ParseURLWithContext(feedURL(config), ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("rss: parse feed: %w", err)
	}
	var since time.Time
	if cursor != nil {
		since = cursor.LastSyncTime
	}
	limit := maxItems(config)
	var items []types.FetchedItem
	for i, fi := range feed.Items {
		if limit > 0 && i >= limit {
			break
		}
		pubAt := itemTime(fi)
		if !since.IsZero() && !pubAt.IsZero() && !pubAt.After(since) {
			continue
		}
		items = append(items, feedItemToFetchedItem(fi))
	}
	nextCursor := &types.SyncCursor{
		LastSyncTime:    time.Now(),
		ConnectorCursor: map[string]interface{}{},
	}
	return items, nextCursor, nil
}

// --- helpers ---

func feedItemToFetchedItem(fi *gofeed.Item) types.FetchedItem {
	content := fi.Content
	if content == "" {
		content = fi.Description
	}
	return types.FetchedItem{
		ExternalID:  itemID(fi),
		Title:       fi.Title,
		Content:     []byte(strings.TrimSpace(content)),
		ContentType: "text/html",
		FileName:    safeFilename(fi.Title) + ".md",
		URL:         fi.Link,
		UpdatedAt:   itemTime(fi),
		Metadata:    map[string]string{"source_url": fi.Link},
	}
}

func feedURL(config *types.DataSourceConfig) string {
	if config == nil || config.Settings == nil {
		return ""
	}
	v, _ := config.Settings["feed_url"].(string)
	return v
}

func maxItems(config *types.DataSourceConfig) int {
	if config == nil || config.Settings == nil {
		return 100
	}
	if v, ok := config.Settings["max_items"].(float64); ok && v > 0 {
		return int(v)
	}
	return 100
}

func itemID(fi *gofeed.Item) string {
	if fi.GUID != "" {
		return fi.GUID
	}
	src := fi.Link
	if src == "" {
		src = fi.Title
	}
	h := sha256.Sum256([]byte(src))
	return hex.EncodeToString(h[:8])
}

func itemTime(fi *gofeed.Item) time.Time {
	if fi.UpdatedParsed != nil {
		return *fi.UpdatedParsed
	}
	if fi.PublishedParsed != nil {
		return *fi.PublishedParsed
	}
	return time.Time{}
}

func safeFilename(s string) string {
	r := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "-", "?", "-", `"`, "-", "<", "-", ">", "-", "|", "-")
	result := r.Replace(s)
	if len(result) > 100 {
		result = result[:100]
	}
	return result
}
```

- [ ] **Step 3: 写入 connector_test.go**

```go
// internal/datasource/connector/rss/connector_test.go
package rss_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/datasource/connector/rss"
	"github.com/Tencent/WeKnora/internal/types"
)

const sampleFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <item>
      <title>Article One</title>
      <link>https://example.com/1</link>
      <guid>guid-1</guid>
      <description>Content of article one</description>
      <pubDate>Mon, 01 Jan 2024 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Article Two</title>
      <link>https://example.com/2</link>
      <guid>guid-2</guid>
      <description>Content of article two</description>
      <pubDate>Tue, 02 Jan 2024 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

func feedServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(sampleFeed))
	}))
}

func TestFetchAllReturnsBothItems(t *testing.T) {
	srv := feedServer(t)
	defer srv.Close()

	c := rss.New()
	config := &types.DataSourceConfig{Settings: map[string]interface{}{"feed_url": srv.URL}}
	items, err := c.FetchAll(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("FetchAll error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ExternalID != "guid-1" {
		t.Errorf("ExternalID = %q, want %q", items[0].ExternalID, "guid-1")
	}
}

func TestFetchIncrementalSkipsPastItems(t *testing.T) {
	srv := feedServer(t)
	defer srv.Close()

	c := rss.New()
	config := &types.DataSourceConfig{Settings: map[string]interface{}{"feed_url": srv.URL}}
	// Set cursor far in the future so both sample items are in the past
	cursor := &types.SyncCursor{LastSyncTime: time.Now().AddDate(1, 0, 0)}
	items, _, err := c.FetchIncremental(context.Background(), config, cursor)
	if err != nil {
		t.Fatalf("FetchIncremental error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items with future cursor, got %d", len(items))
	}
}
```

- [ ] **Step 4: 确认编译和测试**

```bash
make build 2>&1 | head -20
go test ./internal/datasource/connector/rss/... -v
```

Expected: build 成功，tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/datasource/connector/rss/ go.mod go.sum
git commit -m "feat(connector): add RSS/Atom feed connector using gofeed"
```

---

## Task 6: 注册 Web/RSS Connector

**Files:**
- Modify: `internal/container/container.go`

- [ ] **Step 1: 确认现有 Feishu connector 注册位置**

```bash
grep -n "feishu\|Register\|ConnectorRegistry" internal/container/container.go | head -10
```

记录注册代码所在行。

- [ ] **Step 2: 在 container.go 中注册新连接器**

在现有 Feishu connector 注册处之后，添加（与现有 import 结构一致）：

```go
import (
    // ... existing imports ...
    webconnector "github.com/Tencent/WeKnora/internal/datasource/connector/web"
    rssconnector "github.com/Tencent/WeKnora/internal/datasource/connector/rss"
)
```

在 registry 注册调用处：

```go
if err := registry.Register(webconnector.New()); err != nil {
    return fmt.Errorf("register web connector: %w", err)
}
if err := registry.Register(rssconnector.New()); err != nil {
    return fmt.Errorf("register rss connector: %w", err)
}
```

- [ ] **Step 3: 确认编译**

```bash
make build 2>&1 | head -20
```

Expected: build 成功。

- [ ] **Step 4: 验证连接器出现在列表接口**

```bash
make run &
# 查找实际 connector list 接口 — 参考 datasource handler
grep -n "connectors\|ListConnectors" internal/handler/datasource.go | head -5
# 然后调用对应端点
curl -s http://localhost:8080/api/data-sources/connectors | python -m json.tool
```

Expected: 响应包含 `"type": "web_crawler"` 和 `"type": "rss"` 条目。

- [ ] **Step 5: Commit**

```bash
git add internal/container/container.go
git commit -m "feat(container): register Web Crawler and RSS connectors"
```

---

## Self-Review

### Spec coverage

| 规格要求 | 对应 Task |
|---------|----------|
| data_sources.config AES-256 加密 | Task 3 |
| 远端消失 → archived 归档（不删除） | Task 1 |
| external_id 快速查找索引 | Task 2 |
| Web/RSS 连接器实现 | Task 4, Task 5 |
| 连接器注册 + 验收 | Task 6 |

### Placeholder scan

无 TBD/TODO/填写说明。所有 step 均含完整实现代码或可执行命令。

### Type consistency

- `web.Connector` 和 `rss.Connector` 均实现 `datasource.Connector` 接口（Type/Validate/ListResources/FetchAll/FetchIncremental 5 个方法）
- `types.FetchedItem.ExternalID`（已有字段）对应 Task 2 新增的 `knowledge.external_id` 列
- `ManualKnowledgeStatusArchived = "archived"`（Task 1 定义）在 Task 1 Step 5 的 `UpdateStatus` 调用中使用
- `crypto.Encrypt`/`crypto.Decrypt`（Task 3 Step 3 定义）在 Task 3 Step 6 的 `SaveConfig`/`ParseConfig` 中使用
- `configcrypto` 别名（Task 3 Step 6）避免与 Go 内置 `crypto` 包命名冲突

### 注意事项

- **knowledge 字段名**：Task 1 Step 4 中使用 `parse_status` 字段名，需运行 `grep -n "parse_status\|ParseStatus" internal/types/knowledge.go` 确认实际列名后调整
- **gofeed 网络访问**：RSS connector test 使用 httptest.Server，不访问真实网络
- **config 向后兼容**：`DATA_SOURCE_CONFIG_KEY` 未设置时，`ParseConfig` 走明文 JSONB 路径，现有 Feishu 记录不受影响
