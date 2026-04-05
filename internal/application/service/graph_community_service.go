// Package service — graph community summarisation for the GraphRAG pipeline.
//
// After the entity-extraction step builds a knowledge graph in Neo4j, running
// Leiden over that graph groups densely-connected entities into communities.
// The digest of a community ("these entities and relations form a cluster
// about X") is a retrieval artefact at a higher level of abstraction than
// individual chunks — a query that only mentions one entity can surface the
// whole cluster's summary.
//
// This service keeps the two concerns the caller needs at the public edge:
//
//   - BuildSummaries(ctx, namespace) runs detection and renders deterministic
//     text summaries of the top-N communities. The summaries are pure
//     structural renderings (entity names + edge list) so this works even
//     without an LLM wired in; callers that want higher-quality prose can
//     post-process the rendered text with a chat model.
//
//   - FormatForPrompt(summaries) turns a slice of summaries into a single
//     block of context suitable for appending to a user prompt, with a
//     stable heading so chunk-level retrieval context and community context
//     can be distinguished in traces.
//
// The service is backend-agnostic: it only talks to the graph via the
// RetrieveGraphRepository interface, so alternative graph stores that
// implement DetectCommunities / ListCommunityMembers are drop-in.

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// GraphCommunityService produces GraphRAG-style community summaries over the
// entity graph for a given namespace. It is safe to call with a nil graph
// repository or a backend that does not support community detection — in
// both cases BuildSummaries returns (nil, nil) and logs a warning.
type GraphCommunityService struct {
	graphRepo interfaces.RetrieveGraphRepository
}

// NewGraphCommunityService wires the service. Registered in the DI container.
func NewGraphCommunityService(graphRepo interfaces.RetrieveGraphRepository) *GraphCommunityService {
	return &GraphCommunityService{graphRepo: graphRepo}
}

// CommunitySummary is a single community's rendered digest. Size is copied
// out of the underlying group so consumers can threshold on it without
// re-walking the raw graph data.
type CommunitySummary struct {
	CommunityID int64    `json:"community_id"`
	Size        int      `json:"size"`
	Entities    []string `json:"entities"`
	// Edges are rendered "<source> -[<type>]-> <target>" strings, which is
	// both a stable human-readable form and directly embeddable.
	Edges []string `json:"edges,omitempty"`
	// Text is the full deterministic render (entities + edges) that
	// downstream code should embed or feed to an LLM summariser.
	Text string `json:"text"`
}

// BuildSummariesOptions controls which communities are surfaced. Defaults are
// chosen to keep the summary block small enough to fit into a prompt: the
// top 8 communities by size, with a minimum of 2 members to avoid surfacing
// isolated entity pairs that carry little context.
type BuildSummariesOptions struct {
	TopN    int
	MinSize int
}

// DefaultBuildSummariesOptions returns production-sane defaults.
func DefaultBuildSummariesOptions() BuildSummariesOptions {
	return BuildSummariesOptions{TopN: 8, MinSize: 2}
}

// BuildSummaries runs community detection over the namespace's graph and
// returns rendered summaries for the top-N largest communities.
//
// Detection is idempotent: re-running overwrites the community property on
// each node. Callers that need to refresh stale summaries can simply call
// BuildSummaries again.
func (s *GraphCommunityService) BuildSummaries(
	ctx context.Context,
	namespace types.NameSpace,
	opts BuildSummariesOptions,
) ([]*CommunitySummary, error) {
	if s.graphRepo == nil {
		logger.Warnf(ctx, "graph repo not configured — skipping community summaries")
		return nil, nil
	}
	if opts.TopN <= 0 {
		opts.TopN = DefaultBuildSummariesOptions().TopN
	}
	if opts.MinSize <= 0 {
		opts.MinSize = DefaultBuildSummariesOptions().MinSize
	}

	n, err := s.graphRepo.DetectCommunities(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("detect communities: %w", err)
	}
	logger.Infof(ctx, "community detection produced %d communities for kb=%s knowledge=%s",
		n, namespace.KnowledgeBase, namespace.Knowledge)
	if n == 0 {
		return nil, nil
	}

	groups, err := s.graphRepo.ListCommunityMembers(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("list community members: %w", err)
	}

	out := make([]*CommunitySummary, 0, len(groups))
	for _, g := range groups {
		if g.Size < opts.MinSize {
			continue
		}
		out = append(out, renderCommunitySummary(g))
		if len(out) >= opts.TopN {
			break
		}
	}
	return out, nil
}

// renderCommunitySummary turns a raw CommunityGroup into the stable
// text form the rest of the pipeline embeds or prompts with.
func renderCommunitySummary(g *types.CommunityGroup) *CommunitySummary {
	entities := make([]string, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		if node == nil || node.Name == "" {
			continue
		}
		entities = append(entities, node.Name)
	}
	edges := make([]string, 0, len(g.Relation))
	for _, rel := range g.Relation {
		if rel == nil {
			continue
		}
		edges = append(edges, fmt.Sprintf("%s -[%s]-> %s", rel.Node1, rel.Type, rel.Node2))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Community #%d (%d entities)\n", g.ID, g.Size)
	fmt.Fprintf(&b, "Entities: %s\n", strings.Join(entities, ", "))
	if len(edges) > 0 {
		b.WriteString("Relations:\n")
		for _, e := range edges {
			b.WriteString("  - ")
			b.WriteString(e)
			b.WriteByte('\n')
		}
	}
	return &CommunitySummary{
		CommunityID: g.ID,
		Size:        g.Size,
		Entities:    entities,
		Edges:       edges,
		Text:        b.String(),
	}
}

// FormatForPrompt concatenates summaries into a single labelled block that
// can be appended to a user prompt. Returns an empty string when summaries
// is empty so callers can unconditionally append the result.
func FormatForPrompt(summaries []*CommunitySummary) string {
	if len(summaries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Knowledge Graph Communities\n")
	for _, cs := range summaries {
		b.WriteString(cs.Text)
		b.WriteByte('\n')
	}
	return b.String()
}
