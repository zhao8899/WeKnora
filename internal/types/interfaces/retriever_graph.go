package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// RetrieveGraphRepository is a repository for retrieving graphs
type RetrieveGraphRepository interface {
	// AddGraph adds a graph to the repository
	AddGraph(ctx context.Context, namespace types.NameSpace, graphs []*types.GraphData) error
	// DelGraph deletes a graph from the repository
	DelGraph(ctx context.Context, namespace []types.NameSpace) error
	// SearchNode searches for nodes in the repository
	SearchNode(ctx context.Context, namespace types.NameSpace, nodes []string) (*types.GraphData, error)
	// DetectCommunities runs a community-detection algorithm (Leiden via Neo4j GDS
	// when available) over the sub-graph for this namespace, writing a
	// ``community`` property onto each node. Returns the number of communities
	// discovered. Backends without community detection support should return
	// (0, nil) so callers can degrade gracefully.
	DetectCommunities(ctx context.Context, namespace types.NameSpace) (int, error)
	// ListCommunityMembers groups nodes (and their connecting relations) by the
	// ``community`` property written by DetectCommunities. Groups are returned
	// sorted by descending size. An empty slice means no communities were found
	// (e.g. detection has not been run, the graph is empty, or the backend has
	// no community support).
	ListCommunityMembers(ctx context.Context, namespace types.NameSpace) ([]*types.CommunityGroup, error)
}
