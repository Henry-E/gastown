package refinery

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/deliveryreceipt"
)

// persistMRDeliveryReceipt records proof after a successful push and before
// any MR/source/branch cleanup. observedTargetSHA may be supplied by a batch so
// one fresh remote read binds every member to the same target result;
// expectedTargetSHA is the actual pushed merge result. On a delayed retry the
// remote may have advanced, but the expected result must remain its ancestor.
func (e *Engineer) persistMRDeliveryReceipt(mr *MRInfo, observedTargetSHA, expectedTargetSHA, strategy, batchID string) (*deliveryreceipt.Receipt, error) {
	if mr == nil {
		return nil, fmt.Errorf("merge request metadata is unavailable")
	}
	submittedCommit := strings.TrimSpace(mr.CommitSHA)
	if submittedCommit == "" && mr.Branch != "" {
		resolved, err := e.git.Rev(mr.Branch)
		if err != nil {
			return nil, fmt.Errorf("resolve submitted branch %s: %w", mr.Branch, err)
		}
		submittedCommit = resolved
	}
	attemptID := strings.TrimSpace(mr.AttemptID)
	if attemptID == "" && mr.ID != "" && submittedCommit != "" {
		attemptID = deliveryreceipt.LegacyAttemptID(mr.ID, mr.RetryCount, submittedCommit)
	}
	rigName := strings.TrimSpace(mr.Rig)
	if rigName == "" {
		rigName = e.rig.Name
	}
	target := strings.TrimSpace(mr.Target)
	if target == "" {
		target = e.rig.DefaultBranch()
	}
	targetRef := "refs/heads/" + strings.TrimPrefix(strings.TrimPrefix(target, "refs/heads/"), "origin/")
	expectedTargetSHA = strings.ToLower(strings.TrimSpace(expectedTargetSHA))
	if expectedTargetSHA == "" {
		return nil, fmt.Errorf("expected merge result SHA is required")
	}
	verifyRemoteContains := func(expected string) (string, error) {
		if observedTargetSHA == "" {
			var readErr error
			observedTargetSHA, readErr = e.git.PushRemoteRefSHA("origin", targetRef)
			if readErr != nil {
				return "", fmt.Errorf("fresh remote target read: %w", readErr)
			}
		}
		if strings.EqualFold(observedTargetSHA, expected) {
			return "git_ls_remote_matches_merge_result", nil
		}
		if fetchErr := e.git.FetchPushRemoteRef("origin", targetRef); fetchErr != nil {
			return "", fmt.Errorf("fetch current target for merge-result verification: %w", fetchErr)
		}
		ancestor, ancestorErr := e.git.IsAncestor(expected, observedTargetSHA)
		if ancestorErr != nil {
			return "", fmt.Errorf("verify merge result on current target: %w", ancestorErr)
		}
		if !ancestor {
			return "", fmt.Errorf("merge result %s is not on fresh remote target %s", expected, observedTargetSHA)
		}
		return "git_ls_remote_contains_merge_result_after_advance", nil
	}

	identity := deliveryreceipt.Identity{Rig: rigName, SourceBead: mr.SourceIssue, AttemptID: attemptID}
	townRoot := filepath.Dir(e.rig.Path)
	existing, err := deliveryreceipt.Find(townRoot, identity)
	if err != nil {
		return nil, fmt.Errorf("read delivery receipt replay log: %w", err)
	}
	if existing != nil {
		if !strings.EqualFold(existing.FinalTargetSHA, expectedTargetSHA) {
			return nil, fmt.Errorf("receipt %s target %s conflicts with merge checkpoint %s", existing.ReceiptID, existing.FinalTargetSHA, expectedTargetSHA)
		}
		if _, err := verifyRemoteContains(existing.FinalTargetSHA); err != nil {
			return nil, err
		}
		return existing, nil
	}

	verificationMethod, err := verifyRemoteContains(expectedTargetSHA)
	if err != nil {
		return nil, err
	}
	repository, err := e.git.GetPushURL("origin")
	if err != nil {
		return nil, fmt.Errorf("resolve pushed repository: %w", err)
	}
	receipt := deliveryreceipt.Receipt{
		Rig:                rigName,
		Repository:         repository,
		SourceBead:         mr.SourceIssue,
		BarnabyJobID:       mr.BarnabyJobID,
		MRID:               mr.ID,
		AttemptID:          attemptID,
		SubmittedBranch:    mr.Branch,
		SubmittedCommit:    submittedCommit,
		TargetRef:          targetRef,
		FinalTargetSHA:     expectedTargetSHA,
		MergeStrategy:      strategy,
		BatchID:            batchID,
		Producer:           "gt.refinery.engineer",
		VerifiedAt:         time.Now().UTC(),
		VerificationMethod: verificationMethod,
	}
	stored, _, err := deliveryreceipt.Append(townRoot, receipt)
	if err != nil {
		return nil, fmt.Errorf("persist delivery receipt: %w", err)
	}
	return &stored, nil
}
