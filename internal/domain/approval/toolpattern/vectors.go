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

type vectorFixture struct {
	Matches   []MatchVector     `json:"matches"`
	Canonical []CanonicalVector `json:"canonical"`
}

var parsedVectors = sync.OnceValue(func() vectorFixture {
	var vectors vectorFixture
	if err := json.Unmarshal(vectorsJSON, &vectors); err != nil {
		panic(err)
	}
	return vectors
})

func MatchVectors() []MatchVector         { return parsedVectors().Matches }
func CanonicalVectors() []CanonicalVector { return parsedVectors().Canonical }
