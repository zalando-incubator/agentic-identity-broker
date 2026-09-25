package approval

import (
	"fmt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval/toolpattern"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func ApplyExactPatterns(approval *storage.ToolApproval) error {
	toolPattern := toolpattern.EscapeLiteral(approval.ToolName)
	paramsPattern := toolpattern.ExactParams(approval.Arguments)
	if err := toolpattern.ValidateToolPattern(toolPattern); err != nil {
		return fmt.Errorf("%w: %w", ErrApprovalInvalidPattern, err)
	}
	if err := toolpattern.ValidateParamsPattern(paramsPattern); err != nil {
		return fmt.Errorf("%w: %w", ErrApprovalInvalidPattern, err)
	}
	approval.ToolPattern = toolPattern
	approval.ParamsPattern = paramsPattern
	return nil
}
