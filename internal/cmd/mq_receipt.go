package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/deliveryreceipt"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/rig"
)

var errDeliveryReceiptPersistence = errors.New("delivery receipt persistence failed")

// persistMQDeliveryReceipt is the producer-side commit point for the live
// refinery path. The merge has already pushed; this command freshly rereads
// the remote and requires it to equal the merge result supplied by that path.
// It is intentionally separate from post-merge cleanup, which only consumes
// an already-durable receipt.
func persistMQDeliveryReceipt(townRoot, rigName string, r *rig.Rig, mr *refinery.MergeRequest, expectedTargetSHA, mergeMode string) (*deliveryreceipt.Receipt, error) {
	identity, attemptID, err := mqDeliveryIdentity(rigName, mr)
	if err != nil {
		return nil, err
	}
	expectedTargetSHA = strings.ToLower(strings.TrimSpace(expectedTargetSHA))
	if expectedTargetSHA == "" {
		return nil, errors.New("expected target SHA is required")
	}

	existing, err := deliveryreceipt.Find(townRoot, identity)
	if err != nil {
		return nil, fmt.Errorf("%w: reading replay log: %v", errDeliveryReceiptPersistence, err)
	}
	if existing != nil {
		return existing, nil
	}

	rigGit, err := getRigGit(r.Path)
	if err != nil {
		return nil, fmt.Errorf("opening rig repository for remote verification: %w", err)
	}
	targetBranch := mqTargetBranch(r, mr)
	targetRef := "refs/heads/" + targetBranch
	observedTargetSHA, err := rigGit.PushRemoteRefSHA("origin", targetRef)
	if err != nil {
		return nil, fmt.Errorf("fresh remote target read failed: %w", err)
	}
	verificationMethod := "git_ls_remote_matches_supplied_merge_result"
	if !strings.EqualFold(observedTargetSHA, expectedTargetSHA) {
		if err := rigGit.FetchPushRemoteRef("origin", targetRef); err != nil {
			return nil, fmt.Errorf("fetch current target for merge-result verification: %w", err)
		}
		ancestor, err := rigGit.IsAncestor(expectedTargetSHA, observedTargetSHA)
		if err != nil {
			return nil, fmt.Errorf("verify supplied merge result on current target: %w", err)
		}
		if !ancestor {
			return nil, fmt.Errorf("supplied merge result %s is not on fresh remote target %s", expectedTargetSHA, observedTargetSHA)
		}
		verificationMethod = "git_ls_remote_contains_supplied_merge_result_after_advance"
	}

	mode := strings.ToLower(strings.TrimSpace(mergeMode))
	if mode == "" {
		mode = postMergeConfiguredMode(r.Path)
	}
	if mode != "direct" && mode != "pr" {
		return nil, fmt.Errorf("unsupported refinery merge mode %q", mode)
	}
	if mode != "pr" {
		localTargetSHA, revErr := rigGit.Rev(targetRef)
		if revErr != nil {
			return nil, fmt.Errorf("resolve local merged target %s: %w", targetRef, revErr)
		}
		localContainsExpected := strings.EqualFold(localTargetSHA, expectedTargetSHA)
		if !localContainsExpected {
			localContainsExpected, err = rigGit.IsAncestor(expectedTargetSHA, localTargetSHA)
			if err != nil {
				return nil, fmt.Errorf("verify merge result on local target %s: %w", targetRef, err)
			}
		}
		if !localContainsExpected {
			return nil, fmt.Errorf("supplied merge result %s is not on local merged target %s (%s)", expectedTargetSHA, targetRef, localTargetSHA)
		}
		if strings.EqualFold(observedTargetSHA, expectedTargetSHA) {
			verificationMethod = "git_ls_remote_matches_local_and_supplied_merge_result"
		} else {
			verificationMethod = "git_ls_remote_and_local_contain_supplied_merge_result_after_advance"
		}
	}

	repository, err := rigGit.GetPushURL("origin")
	if err != nil {
		return nil, fmt.Errorf("resolving pushed repository: %w", err)
	}
	receipt := deliveryreceipt.Receipt{
		Rig:                rigName,
		Repository:         repository,
		SourceBead:         mr.IssueID,
		BarnabyJobID:       mr.BarnabyJobID,
		MRID:               mr.ID,
		AttemptID:          attemptID,
		SubmittedBranch:    mr.Branch,
		SubmittedCommit:    mr.CommitSHA,
		TargetRef:          targetRef,
		FinalTargetSHA:     expectedTargetSHA,
		MergeStrategy:      postMergeStrategy(r.Path),
		Producer:           "gt.mq.record-delivery",
		VerifiedAt:         time.Now().UTC(),
		VerificationMethod: verificationMethod,
	}
	prepared, err := deliveryreceipt.Prepare(receipt)
	if err != nil {
		return nil, fmt.Errorf("invalid delivery receipt: %w", err)
	}
	stored, _, err := deliveryreceipt.Append(townRoot, prepared)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errDeliveryReceiptPersistence, err)
	}
	return &stored, nil
}

// validatePostMergeReceipt authorizes destructive cleanup from an existing
// producer receipt. It never creates a receipt. The remote may have advanced
// since delivery, so an older receipt remains valid when its target SHA is an
// ancestor of the freshly read current target.
func validatePostMergeReceipt(townRoot, rigName string, r *rig.Rig, mr *refinery.MergeRequest) (*deliveryreceipt.Receipt, error) {
	identity, _, err := mqDeliveryIdentity(rigName, mr)
	if err != nil {
		return nil, err
	}
	receipt, err := deliveryreceipt.Find(townRoot, identity)
	if err != nil {
		return nil, fmt.Errorf("%w: reading replay log: %v", errDeliveryReceiptPersistence, err)
	}
	if receipt == nil {
		return nil, fmt.Errorf("no durable delivery receipt for rig=%s source=%s attempt=%s", identity.Rig, identity.SourceBead, identity.AttemptID)
	}
	targetRef := "refs/heads/" + mqTargetBranch(r, mr)
	if receipt.MRID != mr.ID || receipt.SubmittedBranch != mr.Branch || !strings.EqualFold(receipt.SubmittedCommit, mr.CommitSHA) || receipt.TargetRef != targetRef {
		return nil, fmt.Errorf("delivery receipt %s does not match current MR submission", receipt.ReceiptID)
	}

	rigGit, err := getRigGit(r.Path)
	if err != nil {
		return nil, fmt.Errorf("opening rig repository for receipt validation: %w", err)
	}
	currentTargetSHA, err := rigGit.PushRemoteRefSHA("origin", targetRef)
	if err != nil {
		return nil, fmt.Errorf("fresh remote target read failed: %w", err)
	}
	if strings.EqualFold(receipt.FinalTargetSHA, currentTargetSHA) {
		return receipt, nil
	}
	if err := rigGit.FetchPushRemoteRef("origin", targetRef); err != nil {
		return nil, fmt.Errorf("fetch current target for receipt ancestry: %w", err)
	}
	ancestor, err := rigGit.IsAncestor(receipt.FinalTargetSHA, currentTargetSHA)
	if err != nil {
		return nil, fmt.Errorf("validate receipt target ancestry: %w", err)
	}
	if !ancestor {
		return nil, fmt.Errorf("receipt target %s is not on freshly read remote target %s", receipt.FinalTargetSHA, currentTargetSHA)
	}
	return receipt, nil
}

func mqDeliveryIdentity(rigName string, mr *refinery.MergeRequest) (deliveryreceipt.Identity, string, error) {
	if mr == nil {
		return deliveryreceipt.Identity{}, "", errors.New("merge request metadata is unavailable")
	}
	attemptID := strings.TrimSpace(mr.AttemptID)
	if attemptID == "" && mr.ID != "" && mr.CommitSHA != "" {
		attemptID = deliveryreceipt.LegacyAttemptID(mr.ID, mr.RetryCount, mr.CommitSHA)
	}
	missing := make([]string, 0, 4)
	if strings.TrimSpace(mr.IssueID) == "" {
		missing = append(missing, "source_bead")
	}
	if strings.TrimSpace(mr.Branch) == "" {
		missing = append(missing, "submitted_branch")
	}
	if strings.TrimSpace(mr.CommitSHA) == "" {
		missing = append(missing, "submitted_commit")
	}
	if attemptID == "" {
		missing = append(missing, "attempt_id")
	}
	if len(missing) > 0 {
		return deliveryreceipt.Identity{}, "", fmt.Errorf("MR %s lacks receipt fields: %s", mr.ID, strings.Join(missing, ", "))
	}
	identity := deliveryreceipt.Identity{Rig: rigName, SourceBead: mr.IssueID, AttemptID: attemptID}
	return identity, attemptID, nil
}

func mqTargetBranch(r *rig.Rig, mr *refinery.MergeRequest) string {
	targetBranch := strings.TrimSpace(mr.TargetBranch)
	if targetBranch == "" {
		targetBranch = r.DefaultBranch()
	}
	return strings.TrimPrefix(strings.TrimPrefix(targetBranch, "refs/heads/"), "origin/")
}

func postMergeReceiptStrict(mr *refinery.MergeRequest) bool {
	_ = mr // retained for call-site clarity and future per-MR rollout controls
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GT_DELIVERY_RECEIPT_ENFORCE"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func postMergeConfiguredMode(rigPath string) string {
	settingsPath := filepath.Join(rigPath, "settings", "config.json")
	if settings, err := config.LoadRigSettings(settingsPath); err == nil && settings.MergeQueue != nil {
		if mode := strings.ToLower(strings.TrimSpace(settings.MergeQueue.MergeStrategy)); mode != "" {
			return mode
		}
	}
	return "direct"
}

func postMergeStrategy(rigPath string) string {
	// Both live formula modes rebase the submitted work before either a direct
	// fast-forward push or a hosted merge.
	_ = rigPath
	return "rebase"
}
