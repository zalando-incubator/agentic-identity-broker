package approval

import (
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval/toolpattern"
)

type candidateRank struct {
	exactTool         bool
	constrainedParams int
	wildcards         int
}

func specificity(toolPattern string, paramsPattern map[string]string) candidateRank {
	rank := candidateRank{exactTool: countWildcards(toolPattern) == 0, wildcards: countWildcards(toolPattern)}
	for _, pattern := range paramsPattern {
		if pattern != "*" {
			rank.constrainedParams++
		}
		rank.wildcards += countWildcards(pattern)
	}
	return rank
}

func countWildcards(pattern string) int {
	count := 0
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' && i+1 < len(pattern) {
			i++
			continue
		}
		if pattern[i] == '*' {
			count++
		}
	}
	return count
}

func compareRank(a, b candidateRank) int {
	if a.exactTool != b.exactTool {
		if a.exactTool {
			return 1
		}
		return -1
	}
	if a.constrainedParams != b.constrainedParams {
		if a.constrainedParams > b.constrainedParams {
			return 1
		}
		return -1
	}
	if a.wildcards != b.wildcards {
		if a.wildcards < b.wildcards {
			return 1
		}
		return -1
	}
	return 0
}

type candidate struct {
	id            string
	toolPattern   string
	paramsPattern map[string]string
	decidedAt     time.Time
}

func selectBest(candidates []candidate, toolName string, args map[string]any) int {
	best := -1
	for i, candidate := range candidates {
		if !toolpattern.Matches(candidate.toolPattern, candidate.paramsPattern, toolName, args) {
			continue
		}
		if best == -1 {
			best = i
			continue
		}
		comparison := compareRank(specificity(candidate.toolPattern, candidate.paramsPattern), specificity(candidates[best].toolPattern, candidates[best].paramsPattern))
		if comparison > 0 || (comparison == 0 && (candidate.decidedAt.After(candidates[best].decidedAt) || (candidate.decidedAt.Equal(candidates[best].decidedAt) && candidate.id < candidates[best].id))) {
			best = i
		}
	}
	return best
}
