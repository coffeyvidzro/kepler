package security

import "context"

// Runtime evaluates security policies for semantic application actions.
type Runtime interface {
	Evaluate(ctx context.Context, evaluation Evaluation) (Decision, error)
}
