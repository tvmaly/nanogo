package contracts

import "context"

type ApprovalGate interface {
	RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalResult, error)
}

type ApprovalRequest struct {
	ID, SessionID, Reason, Summary string
	Data                           map[string]any
	Metadata                       map[string]string
}

type ApprovalResult struct {
	Approved, Rejected bool
	Comment            string
	Metadata           map[string]string
}
