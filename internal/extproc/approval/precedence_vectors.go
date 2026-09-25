package approval

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed precedence_vectors.json
var precedenceVectorsJSON []byte

type precedenceVectorCandidate struct {
	ID            string            `json:"id"`
	ToolPattern   string            `json:"tool_pattern"`
	ParamsPattern map[string]string `json:"params_pattern"`
	DecidedAt     string            `json:"decided_at"`
}
type precedenceVector struct {
	Name       string                      `json:"name"`
	Candidates []precedenceVectorCandidate `json:"candidates"`
	ToolName   string                      `json:"tool_name"`
	Arguments  map[string]any              `json:"arguments"`
	WinnerID   string                      `json:"winner_id"`
}

var loadPrecedenceVectors = sync.OnceValue(func() []precedenceVector {
	var vectors []precedenceVector
	if err := json.Unmarshal(precedenceVectorsJSON, &vectors); err != nil {
		panic(err)
	}
	return vectors
})

func precedenceVectors() []precedenceVector { return loadPrecedenceVectors() }
