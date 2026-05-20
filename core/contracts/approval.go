package contracts

import "context"

type ApprovalGate interface {
	RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalResult, error)
}

type ApprovalRequest struct {
	ID        string
	SessionID string
	Reason    string
	Summary   string
	Data      map[string]any
	Metadata  map[string]string
}

type ApprovalResult struct {
	Approved bool
	Rejected bool
	Comment  string
	Metadata map[string]string
}
