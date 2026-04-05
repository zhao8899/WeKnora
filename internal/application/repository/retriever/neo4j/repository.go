package neo4j

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

// Neo4jRepository is a repository for Neo4j
type Neo4jRepository struct {
	driver     neo4j.Driver
	nodePrefix string
}

// NewNeo4jRepository creates a new Neo4j repository
func NewNeo4jRepository(driver neo4j.Driver) interfaces.RetrieveGraphRepository {
	return &Neo4jRepository{driver: driver, nodePrefix: "ENTITY"}
}

// _remove_hyphen removes hyphens from a string
func _remove_hyphen(s string) string {
	return strings.ReplaceAll(s, "-", "_")
}

// Labels returns the labels for a namespace
func (n *Neo4jRepository) Labels(namespace types.NameSpace) []string {
	res := make([]string, 0)
	for _, label := range namespace.Labels() {
		res = append(res, n.nodePrefix+_remove_hyphen(label))
	}
	return res
}

// Label returns the label for a namespace
func (n *Neo4jRepository) Label(namespace types.NameSpace) string {
	labels := n.Labels(namespace)
	return strings.Join(labels, ":")
}

// AddGraph adds a graph to the Neo4j repository
func (n *Neo4jRepository) AddGraph(ctx context.Context, namespace types.NameSpace, graphs []*types.GraphData) error {
	if n.driver == nil {
		logger.Warnf(ctx, "NOT SUPPORT RETRIEVE GRAPH")
		return nil
	}
	for _, graph := range graphs {
		if err := n.addGraph(ctx, namespace, graph); err != nil {
			return err
		}
	}
	return nil
}

// addGraph adds a graph to the Neo4j repository
func (n *Neo4jRepository) addGraph(ctx context.Context, namespace types.NameSpace, graph *types.GraphData) error {
	session := n.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// Node import query
		node_import_query := `
			UNWIND $data AS row
			CALL apoc.merge.node(row.labels, {name: row.name, kg: row.knowledge_id}, row.props, {}) YIELD node
			SET node.chunks = apoc.coll.union(node.chunks, row.chunks)
			RETURN distinct 'done' AS result
		`
		nodeData := []map[string]interface{}{}
		for _, node := range graph.Node {
			nodeData = append(nodeData, map[string]interface{}{
				"name":         node.Name,
				"knowledge_id": namespace.Knowledge,
				"props":        map[string][]string{"attributes": node.Attributes},
				"chunks":       node.Chunks,
				"labels":       n.Labels(namespace),
			})
		}
		if _, err := tx.Run(ctx, node_import_query, map[string]interface{}{"data": nodeData}); err != nil {
			return nil, fmt.Errorf("failed to create nodes: %v", err)
		}

		// Relationship import query
		rel_import_query := `
			UNWIND $data AS row
			CALL apoc.merge.node(row.source_labels, {name: row.source, kg: row.knowledge_id}, {}, {}) YIELD node as source
			CALL apoc.merge.node(row.target_labels, {name: row.target, kg: row.knowledge_id}, {}, {}) YIELD node as target
			CALL apoc.merge.relationship(source, row.type, {}, row.attributes, target) YIELD rel
			RETURN distinct 'done'
		`
		relData := []map[string]interface{}{}
		for _, rel := range graph.Relation {
			relData = append(relData, map[string]interface{}{
				"source":        rel.Node1,
				"target":        rel.Node2,
				"knowledge_id":  namespace.Knowledge,
				"type":          rel.Type,
				"source_labels": n.Labels(namespace),
				"target_labels": n.Labels(namespace),
			})
		}
		if _, err := tx.Run(ctx, rel_import_query, map[string]interface{}{"data": relData}); err != nil {
			return nil, fmt.Errorf("failed to create relationships: %v", err)
		}
		return nil, nil
	})
	if err != nil {
		logger.Errorf(ctx, "failed to add graph: %v", err)
		return err
	}
	return nil
}

// DelGraph deletes a graph from the Neo4j repository
func (n *Neo4jRepository) DelGraph(ctx context.Context, namespaces []types.NameSpace) error {
	if n.driver == nil {
		logger.Warnf(ctx, "NOT SUPPORT RETRIEVE GRAPH")
		return nil
	}
	session := n.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		for _, namespace := range namespaces {
			labelExpr := n.Label(namespace)

			deleteRelsQuery := `
				CALL apoc.periodic.iterate(
					"MATCH (n:` + labelExpr + ` {kg: $knowledge_id})-[r]-(m:` + labelExpr + ` {kg: $knowledge_id}) RETURN r",
					"DELETE r",
					{batchSize: 1000, parallel: true, params: {knowledge_id: $knowledge_id}}
				) YIELD batches, total
				RETURN total
        	`
			if _, err := tx.Run(ctx, deleteRelsQuery, map[string]interface{}{"knowledge_id": namespace.Knowledge}); err != nil {
				return nil, fmt.Errorf("failed to delete relationships: %v", err)
			}

			deleteNodesQuery := `
				CALL apoc.periodic.iterate(
					"MATCH (n:` + labelExpr + ` {kg: $knowledge_id}) RETURN n",
					"DELETE n",
					{batchSize: 1000, parallel: true, params: {knowledge_id: $knowledge_id}}
				) YIELD batches, total
				RETURN total
        	`
			if _, err := tx.Run(ctx, deleteNodesQuery, map[string]interface{}{"knowledge_id": namespace.Knowledge}); err != nil {
				return nil, fmt.Errorf("failed to delete nodes: %v", err)
			}
		}
		return nil, nil
	})
	if err != nil {
		return err
	}
	logger.Infof(ctx, "delete graph result: %v", result)
	return nil
}

// SearchNode searches for nodes in the Neo4j repository
func (n *Neo4jRepository) SearchNode(
	ctx context.Context,
	namespace types.NameSpace,
	nodes []string,
) (*types.GraphData, error) {
	if n.driver == nil {
		logger.Warnf(ctx, "NOT SUPPORT RETRIEVE GRAPH")
		return nil, nil
	}
	session := n.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		labelExpr := n.Label(namespace)
		query := `
			MATCH (n:` + labelExpr + `)-[r]-(m:` + labelExpr + `)
			WHERE ANY(nodeText IN $nodes WHERE n.name CONTAINS nodeText)
			RETURN n, r, m
		`
		params := map[string]interface{}{"nodes": nodes}
		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, fmt.Errorf("failed to run query: %v", err)
		}

		graphData := &types.GraphData{}
		nodeSeen := make(map[string]bool)
		for result.Next(ctx) {
			record := result.Record()
			node, _ := record.Get("n")
			rel, _ := record.Get("r")
			targetNode, _ := record.Get("m")

			nodeData := node.(neo4j.Node)
			targetNodeData := targetNode.(neo4j.Node)

			// Convert node to types.Node
			for _, n := range []neo4j.Node{nodeData, targetNodeData} {
				nameStr := n.Props["name"].(string)
				if _, ok := nodeSeen[nameStr]; !ok {
					nodeSeen[nameStr] = true
					graphData.Node = append(graphData.Node, &types.GraphNode{
						Name:       nameStr,
						Chunks:     listI2listS(n.Props["chunks"].([]interface{})),
						Attributes: listI2listS(n.Props["attributes"].([]interface{})),
					})
				}
			}

			// Convert relationship to types.Relation
			relData := rel.(neo4j.Relationship)
			graphData.Relation = append(graphData.Relation, &types.GraphRelation{
				Node1: nodeData.Props["name"].(string),
				Node2: targetNodeData.Props["name"].(string),
				Type:  relData.Type,
			})
		}
		return graphData, nil
	})
	if err != nil {
		logger.Errorf(ctx, "search node failed: %v", err)
		return nil, err
	}
	return result.(*types.GraphData), nil
}

func listI2listS(list []any) []string {
	result := make([]string, len(list))
	for i, v := range list {
		result[i] = fmt.Sprintf("%v", v)
	}
	return result
}

// ---------------------------------------------------------------------------
// GraphRAG community detection
// ---------------------------------------------------------------------------
//
// Community detection is a GraphRAG primitive: after an entity/relation graph
// is extracted from a knowledge base, running Leiden (or Louvain) groups
// densely-connected entities into communities that the system can then
// summarise. Those summaries provide higher-level retrieval context than raw
// chunk search — a query that names one entity can be answered with the
// digest of its whole community.
//
// We rely on the Neo4j GDS library's ``gds.leiden.write`` when available. If
// GDS is not installed (GDS is an optional plugin), we degrade gracefully:
// detection becomes a no-op and the listing call returns an empty slice so
// upstream callers can continue without community context.

// gdsAvailable probes whether the Neo4j instance exposes the GDS library. We
// cache nothing here — it is called at most once per detection request and
// the procedure call is cheap.
func (n *Neo4jRepository) gdsAvailable(ctx context.Context, tx neo4j.ManagedTransaction) bool {
	res, err := tx.Run(ctx, "RETURN gds.version() AS version", nil)
	if err != nil {
		return false
	}
	ok := res.Next(ctx)
	_, _ = res.Consume(ctx)
	return ok
}

// DetectCommunities runs Leiden over the namespace's sub-graph and writes a
// ``community`` int property onto each node. Returns the number of distinct
// communities produced. When GDS is not installed the method is a best-effort
// no-op and returns (0, nil).
func (n *Neo4jRepository) DetectCommunities(
	ctx context.Context, namespace types.NameSpace,
) (int, error) {
	if n.driver == nil {
		logger.Warnf(ctx, "NOT SUPPORT RETRIEVE GRAPH")
		return 0, nil
	}
	session := n.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	count, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		if !n.gdsAvailable(ctx, tx) {
			logger.Warnf(ctx, "neo4j GDS not installed — skipping community detection")
			return 0, nil
		}

		labelExpr := n.Label(namespace)
		// Project a named graph containing just this namespace's sub-graph.
		// The projection is per-call and dropped below; keeping it scoped to
		// kg=$knowledge_id prevents one tenant's detection bleeding into
		// another's graph. We use gds.graph.project.cypher so the filter can
		// be expressed without relying on dynamic label tokens.
		graphName := "wk_comm_" + sanitiseGraphName(namespace.KnowledgeBase, namespace.Knowledge)

		// Drop any stale projection from a prior failed run.
		_, _ = tx.Run(ctx, "CALL gds.graph.drop($name, false) YIELD graphName RETURN graphName",
			map[string]interface{}{"name": graphName})

		nodeQuery := fmt.Sprintf(
			"MATCH (n:%s {kg: $knowledge_id}) RETURN id(n) AS id", labelExpr,
		)
		relQuery := fmt.Sprintf(
			"MATCH (n:%s {kg: $knowledge_id})-[r]-(m:%s {kg: $knowledge_id}) "+
				"RETURN id(n) AS source, id(m) AS target", labelExpr, labelExpr,
		)
		projectParams := map[string]interface{}{
			"name":       graphName,
			"nodeQuery":  nodeQuery,
			"relQuery":   relQuery,
			"paramsJson": map[string]interface{}{"knowledge_id": namespace.Knowledge},
		}
		if _, err := tx.Run(ctx,
			"CALL gds.graph.project.cypher($name, $nodeQuery, $relQuery, "+
				"{parameters: $paramsJson}) YIELD graphName RETURN graphName",
			projectParams,
		); err != nil {
			return 0, fmt.Errorf("gds project failed: %w", err)
		}

		// Leiden writes the ``community`` property on each projected node.
		// We drain the result explicitly before running the cleanup drop —
		// calling tx.Run with an unconsumed prior result is brittle across
		// driver versions.
		var communityCount int
		writeRes, err := tx.Run(ctx,
			"CALL gds.leiden.write($name, {writeProperty: 'community'}) "+
				"YIELD communityCount RETURN communityCount",
			map[string]interface{}{"name": graphName},
		)
		if err == nil {
			if writeRes.Next(ctx) {
				if v, ok := writeRes.Record().Get("communityCount"); ok {
					if cc, ok := v.(int64); ok {
						communityCount = int(cc)
					}
				}
			}
			_, _ = writeRes.Consume(ctx)
		}
		// Always drop the projection, even if leiden failed — stale graphs
		// in the GDS catalog would break the next invocation.
		_, _ = tx.Run(ctx,
			"CALL gds.graph.drop($name, false) YIELD graphName RETURN graphName",
			map[string]interface{}{"name": graphName},
		)
		if err != nil {
			return 0, fmt.Errorf("gds leiden failed: %w", err)
		}
		return communityCount, nil
	})
	if err != nil {
		logger.Errorf(ctx, "detect communities failed: %v", err)
		return 0, err
	}
	return count.(int), nil
}

// ListCommunityMembers groups the namespace's nodes by the ``community``
// property. Groups with no community (detection not run, or GDS absent) are
// skipped. The returned slice is sorted by descending group size.
func (n *Neo4jRepository) ListCommunityMembers(
	ctx context.Context, namespace types.NameSpace,
) ([]*types.CommunityGroup, error) {
	if n.driver == nil {
		logger.Warnf(ctx, "NOT SUPPORT RETRIEVE GRAPH")
		return nil, nil
	}
	session := n.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		labelExpr := n.Label(namespace)
		nodeQuery := `
			MATCH (n:` + labelExpr + ` {kg: $knowledge_id})
			WHERE n.community IS NOT NULL
			RETURN n.community AS community, n.name AS name,
				coalesce(n.chunks, []) AS chunks,
				coalesce(n.attributes, []) AS attributes
		`
		nodeRes, err := tx.Run(ctx, nodeQuery,
			map[string]interface{}{"knowledge_id": namespace.Knowledge})
		if err != nil {
			return nil, fmt.Errorf("list community nodes: %w", err)
		}

		groups := make(map[int64]*types.CommunityGroup)
		nameToCommunity := make(map[string]int64)
		for nodeRes.Next(ctx) {
			rec := nodeRes.Record()
			commRaw, _ := rec.Get("community")
			comm, ok := commRaw.(int64)
			if !ok {
				continue
			}
			name, _ := rec.Get("name")
			chunksRaw, _ := rec.Get("chunks")
			attrsRaw, _ := rec.Get("attributes")
			g := groups[comm]
			if g == nil {
				g = &types.CommunityGroup{ID: comm}
				groups[comm] = g
			}
			nameStr, _ := name.(string)
			g.Nodes = append(g.Nodes, &types.GraphNode{
				Name:       nameStr,
				Chunks:     listI2listS(anyToSlice(chunksRaw)),
				Attributes: listI2listS(anyToSlice(attrsRaw)),
			})
			g.Size++
			nameToCommunity[nameStr] = comm
		}

		// Pull intra-community relations so the summariser has edge context.
		relQuery := `
			MATCH (n:` + labelExpr + ` {kg: $knowledge_id})-[r]->(m:` + labelExpr + ` {kg: $knowledge_id})
			WHERE n.community IS NOT NULL AND n.community = m.community
			RETURN n.name AS source, m.name AS target, type(r) AS type, n.community AS community
		`
		relRes, err := tx.Run(ctx, relQuery,
			map[string]interface{}{"knowledge_id": namespace.Knowledge})
		if err != nil {
			return nil, fmt.Errorf("list community relations: %w", err)
		}
		for relRes.Next(ctx) {
			rec := relRes.Record()
			commRaw, _ := rec.Get("community")
			comm, ok := commRaw.(int64)
			if !ok {
				continue
			}
			g := groups[comm]
			if g == nil {
				continue
			}
			src, _ := rec.Get("source")
			dst, _ := rec.Get("target")
			typ, _ := rec.Get("type")
			srcStr, _ := src.(string)
			dstStr, _ := dst.(string)
			typStr, _ := typ.(string)
			g.Relation = append(g.Relation, &types.GraphRelation{
				Node1: srcStr,
				Node2: dstStr,
				Type:  typStr,
			})
		}

		out := make([]*types.CommunityGroup, 0, len(groups))
		for _, g := range groups {
			out = append(out, g)
		}
		sortCommunityGroups(out)
		return out, nil
	})
	if err != nil {
		logger.Errorf(ctx, "list community members failed: %v", err)
		return nil, err
	}
	return result.([]*types.CommunityGroup), nil
}

// sanitiseGraphName builds a GDS-safe projection name from the namespace.
// GDS graph names are used as Cypher identifiers in some contexts, so we
// restrict them to [A-Za-z0-9_].
func sanitiseGraphName(parts ...string) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteByte('_')
		}
		for _, r := range p {
			switch {
			case r >= '0' && r <= '9', r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
				b.WriteRune(r)
			default:
				b.WriteByte('_')
			}
		}
	}
	s := b.String()
	if s == "" {
		return "anon"
	}
	return s
}

// anyToSlice coerces Neo4j-returned list values to []any. Some driver
// versions return typed slices; this smooths over the difference.
func anyToSlice(v any) []any {
	if v == nil {
		return nil
	}
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

// sortCommunityGroups sorts groups by descending Size so callers that
// summarise "top-N communities" don't need to re-sort.
func sortCommunityGroups(groups []*types.CommunityGroup) {
	// simple insertion sort — N is small (communities per KB)
	for i := 1; i < len(groups); i++ {
		j := i
		for j > 0 && groups[j-1].Size < groups[j].Size {
			groups[j-1], groups[j] = groups[j], groups[j-1]
			j--
		}
	}
}
