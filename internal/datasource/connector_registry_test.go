package datasource

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestNewConnectorRegistryRegistersBuiltinConnectors(t *testing.T) {
	registry := NewConnectorRegistry()
	for _, connectorType := range []string{
		types.ConnectorTypeFeishu,
		types.ConnectorTypeYuque,
		types.ConnectorTypeRSS,
		types.ConnectorTypeWebCrawler,
	} {
		if _, err := registry.Get(connectorType); err != nil {
			t.Fatalf("expected connector %s to be registered: %v", connectorType, err)
		}
	}
}

func TestListAvailableConnectorsIncludesNewSources(t *testing.T) {
	metas := ListAvailableConnectors()
	found := map[string]ConnectorMetadata{}
	for _, meta := range metas {
		found[meta.Type] = meta
	}
	for _, connectorType := range []string{types.ConnectorTypeYuque} {
		meta, ok := found[connectorType]
		if !ok {
			t.Fatalf("expected metadata for %s", connectorType)
		}
		if !meta.Available {
			t.Fatalf("expected %s to be available", connectorType)
		}
	}
}
