package toolpattern

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed vectors.json
var vectorsJSON []byte

type MatchVector struct {
	Name          string            `json:"name"`
	ToolPattern   string            `json:"tool_pattern"`
	ParamsPattern map[string]string `json:"params_pattern"`
	ToolName      string            `json:"tool_name"`
	Arguments     map[string]any    `json:"arguments"`
	Match         bool              `json:"match"`
}

type CanonicalVector struct {
	Name      string `json:"name"`
	Value     any    `json:"value"`
	Canonical string `json:"canonical"`
}

type PrecedenceCandidate struct {
	ID            string            `json:"id"`
	ToolPattern   string            `json:"tool_pattern"`
	ParamsPattern map[string]string `json:"params_pattern"`
	DecidedAt     string            `json:"decided_at"`
}

type PrecedenceVector struct {
	Name       string                `json:"name"`
	Candidates []PrecedenceCandidate `json:"candidates"`
	ToolName   string                `json:"tool_name"`
	Arguments  map[string]any        `json:"arguments"`
	WinnerID   string                `json:"winner_id"`
}

type vectorFixture struct {
	Matches    []MatchVector      `json:"matches"`
	Canonical  []CanonicalVector  `json:"canonical"`
	Precedence []PrecedenceVector `json:"precedence"`
}

var parsedVectors = sync.OnceValue(func() vectorFixture {
	var vectors vectorFixture
	if err := json.Unmarshal(vectorsJSON, &vectors); err != nil {
		panic(err)
	}
	return vectors
})

func MatchVectors() []MatchVector           { return parsedVectors().Matches }
func CanonicalVectors() []CanonicalVector   { return parsedVectors().Canonical }
func PrecedenceVectors() []PrecedenceVector { return parsedVectors().Precedence }
