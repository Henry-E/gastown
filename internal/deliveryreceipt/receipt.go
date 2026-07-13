// Package deliveryreceipt provides the producer-owned durable proof that a
// merge attempt reached a freshly re-read remote target.
package deliveryreceipt

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/steveyegge/gastown/internal/lock"
)

const (
	SchemaVersion = 1
	storeDirName  = "delivery-receipts"
	storeFileName = "receipts.jsonl"
)

// Receipt is the versioned JSONL wire format consumed by Barnaby.
type Receipt struct {
	ReceiptID          string    `json:"receipt_id"`
	SchemaVersion      int       `json:"schema_version"`
	Rig                string    `json:"rig"`
	Repository         string    `json:"repository"`
	SourceBead         string    `json:"source_bead"`
	BarnabyJobID       string    `json:"barnaby_job_id,omitempty"`
	MRID               string    `json:"mr_id"`
	AttemptID          string    `json:"attempt_id"`
	SubmittedBranch    string    `json:"submitted_branch"`
	SubmittedCommit    string    `json:"submitted_commit"`
	TargetRef          string    `json:"target_ref"`
	FinalTargetSHA     string    `json:"final_target_sha"`
	MergeStrategy      string    `json:"merge_strategy"`
	BatchID            string    `json:"batch_id,omitempty"`
	Producer           string    `json:"producer"`
	VerifiedAt         time.Time `json:"verified_at"`
	VerificationMethod string    `json:"verification_method"`
}

// Identity is the idempotency key for one immutable submission attempt.
type Identity struct {
	Rig        string
	SourceBead string
	AttemptID  string
}

func (r Receipt) Identity() Identity {
	return Identity{Rig: r.Rig, SourceBead: r.SourceBead, AttemptID: r.AttemptID}
}

// LegacyAttemptID deterministically identifies pre-attempt_id submissions.
func LegacyAttemptID(mrID string, retryCount int, submittedCommit string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x1f%d\x1f%s", mrID, retryCount, strings.ToLower(strings.TrimSpace(submittedCommit)))))
	return fmt.Sprintf("legacy:%x", sum[:])
}

// StorePath returns the producer replay-log path.
func StorePath(townRoot string) string {
	return filepath.Join(townRoot, storeDirName, storeFileName)
}

// Prepare normalizes, validates, and content-addresses a receipt.
func Prepare(receipt Receipt) (Receipt, error) {
	if receipt.SchemaVersion != 0 && receipt.SchemaVersion != SchemaVersion {
		return Receipt{}, fmt.Errorf("unsupported schema version %d", receipt.SchemaVersion)
	}
	receipt.SchemaVersion = SchemaVersion
	receipt.Rig = strings.TrimSpace(receipt.Rig)
	receipt.Repository = NormalizeRepository(receipt.Repository)
	receipt.SourceBead = strings.TrimSpace(receipt.SourceBead)
	receipt.BarnabyJobID = strings.TrimSpace(receipt.BarnabyJobID)
	receipt.MRID = strings.TrimSpace(receipt.MRID)
	receipt.AttemptID = strings.TrimSpace(receipt.AttemptID)
	receipt.SubmittedBranch = strings.TrimSpace(receipt.SubmittedBranch)
	receipt.SubmittedCommit = strings.ToLower(strings.TrimSpace(receipt.SubmittedCommit))
	receipt.TargetRef = fullBranchRef(receipt.TargetRef)
	receipt.FinalTargetSHA = strings.ToLower(strings.TrimSpace(receipt.FinalTargetSHA))
	receipt.MergeStrategy = strings.ToLower(strings.TrimSpace(receipt.MergeStrategy))
	receipt.BatchID = strings.TrimSpace(receipt.BatchID)
	receipt.Producer = strings.TrimSpace(receipt.Producer)
	receipt.VerificationMethod = strings.TrimSpace(receipt.VerificationMethod)
	receipt.VerifiedAt = receipt.VerifiedAt.UTC()
	receipt.ReceiptID = ""

	if err := validateFacts(receipt); err != nil {
		return Receipt{}, err
	}
	receiptID, err := computeID(receipt)
	if err != nil {
		return Receipt{}, err
	}
	receipt.ReceiptID = receiptID
	return receipt, nil
}

// Append fsyncs a new receipt to the town-root replay log. An exact replay is
// successful and does not append. A conflicting claim for the same identity
// is rejected rather than overwriting prior proof.
func Append(townRoot string, receipt Receipt) (Receipt, bool, error) {
	prepared, err := Prepare(receipt)
	if err != nil {
		return Receipt{}, false, err
	}
	dir, path, err := ensureStore(townRoot)
	if err != nil {
		return Receipt{}, false, err
	}
	unlock, err := lock.FlockAcquire(path + ".lock")
	if err != nil {
		return Receipt{}, false, fmt.Errorf("locking delivery receipt store: %w", err)
	}
	defer unlock()

	existing, err := readAll(path)
	if err != nil {
		return Receipt{}, false, err
	}
	for _, stored := range existing {
		if stored.Identity() != prepared.Identity() {
			continue
		}
		if stored.ReceiptID == prepared.ReceiptID {
			if err := syncFile(path); err != nil {
				return Receipt{}, false, fmt.Errorf("syncing replayed delivery receipt: %w", err)
			}
			if err := syncDirectory(dir); err != nil {
				return Receipt{}, false, fmt.Errorf("syncing replayed delivery receipt directory: %w", err)
			}
			return stored, false, nil
		}
		return Receipt{}, false, fmt.Errorf(
			"conflicting delivery receipt for (%s, %s, %s): existing %s, incoming %s",
			prepared.Rig, prepared.SourceBead, prepared.AttemptID,
			stored.ReceiptID, prepared.ReceiptID,
		)
	}

	data, err := json.Marshal(prepared)
	if err != nil {
		return Receipt{}, false, fmt.Errorf("marshaling delivery receipt: %w", err)
	}
	data = append(data, '\n')
	_, statErr := os.Stat(path)
	fileWasMissing := errors.Is(statErr, os.ErrNotExist)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // operational proof, no secrets
	if err != nil {
		return Receipt{}, false, fmt.Errorf("opening delivery receipt store: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return Receipt{}, false, fmt.Errorf("writing delivery receipt: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return Receipt{}, false, fmt.Errorf("syncing delivery receipt: %w", err)
	}
	if err := f.Close(); err != nil {
		return Receipt{}, false, fmt.Errorf("closing delivery receipt store: %w", err)
	}
	if fileWasMissing {
		if err := syncDirectory(dir); err != nil {
			return Receipt{}, false, fmt.Errorf("syncing delivery receipt directory: %w", err)
		}
	}
	return prepared, true, nil
}

// Find returns a previously persisted receipt for an attempt identity.
func Find(townRoot string, identity Identity) (*Receipt, error) {
	dir, path, err := ensureStore(townRoot)
	if err != nil {
		return nil, err
	}
	unlock, err := lock.FlockAcquire(path + ".lock")
	if err != nil {
		return nil, fmt.Errorf("locking delivery receipt store: %w", err)
	}
	defer unlock()
	receipts, err := readAll(path)
	if err != nil {
		return nil, err
	}
	if len(receipts) > 0 {
		if err := syncFile(path); err != nil {
			return nil, fmt.Errorf("syncing existing delivery receipts: %w", err)
		}
		if err := syncDirectory(dir); err != nil {
			return nil, fmt.Errorf("syncing existing delivery receipt directory: %w", err)
		}
	}
	for i := range receipts {
		if receipts[i].Identity() == identity {
			return &receipts[i], nil
		}
	}
	return nil, nil
}

func ensureStore(townRoot string) (string, string, error) {
	townRoot = strings.TrimSpace(townRoot)
	if townRoot == "" {
		return "", "", errors.New("town root is required for delivery receipt store")
	}
	dir := filepath.Join(townRoot, storeDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("creating delivery receipt directory: %w", err)
	}
	if err := syncDirectory(filepath.Dir(dir)); err != nil {
		return "", "", fmt.Errorf("syncing town root after receipt directory creation: %w", err)
	}
	return dir, filepath.Join(dir, storeFileName), nil
}

func readAll(path string) ([]Receipt, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading delivery receipt store: %w", err)
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		return nil, errors.New("delivery receipt store ends with a partial line")
	}

	var receipts []Receipt
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var receipt Receipt
		if err := json.Unmarshal(scanner.Bytes(), &receipt); err != nil {
			return nil, fmt.Errorf("parsing delivery receipt line %d: %w", line, err)
		}
		if err := validateStored(receipt); err != nil {
			return nil, fmt.Errorf("invalid delivery receipt line %d: %w", line, err)
		}
		receipts = append(receipts, receipt)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning delivery receipt store: %w", err)
	}
	return receipts, nil
}

func validateStored(receipt Receipt) error {
	if receipt.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema version %d", receipt.SchemaVersion)
	}
	storedID := strings.ToLower(strings.TrimSpace(receipt.ReceiptID))
	prepared, err := Prepare(receipt)
	if err != nil {
		return err
	}
	if storedID != prepared.ReceiptID {
		return fmt.Errorf("receipt_id mismatch: stored %s, computed %s", storedID, prepared.ReceiptID)
	}
	return nil
}

func validateFacts(receipt Receipt) error {
	if receipt.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema version %d", receipt.SchemaVersion)
	}
	required := map[string]string{
		"rig":                 receipt.Rig,
		"repository":          receipt.Repository,
		"source_bead":         receipt.SourceBead,
		"mr_id":               receipt.MRID,
		"attempt_id":          receipt.AttemptID,
		"submitted_branch":    receipt.SubmittedBranch,
		"producer":            receipt.Producer,
		"verification_method": receipt.VerificationMethod,
	}
	for name, value := range required {
		if value == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if receipt.BarnabyJobID != "" {
		if _, err := uuid.Parse(receipt.BarnabyJobID); err != nil {
			return fmt.Errorf("invalid barnaby_job_id: %w", err)
		}
	}
	if !isGitOID(receipt.SubmittedCommit) {
		return errors.New("submitted_commit must be a 40- or 64-character hex object ID")
	}
	if !isGitOID(receipt.FinalTargetSHA) {
		return errors.New("final_target_sha must be a 40- or 64-character hex object ID")
	}
	if !strings.HasPrefix(receipt.TargetRef, "refs/heads/") || strings.TrimPrefix(receipt.TargetRef, "refs/heads/") == "" {
		return errors.New("target_ref must be a full branch ref")
	}
	switch receipt.MergeStrategy {
	case "ff", "rebase", "squash", "batch":
	default:
		return fmt.Errorf("unsupported merge_strategy %q", receipt.MergeStrategy)
	}
	if receipt.VerifiedAt.IsZero() {
		return errors.New("verified_at is required")
	}
	return nil
}

func computeID(receipt Receipt) (string, error) {
	receipt.ReceiptID = ""
	data, err := json.Marshal(receipt)
	if err != nil {
		return "", fmt.Errorf("marshaling receipt identity: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func isGitOID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func fullBranchRef(ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "refs/heads/")
	ref = strings.TrimPrefix(ref, "origin/")
	return "refs/heads/" + ref
}

// NormalizeRepository removes URL credentials while preserving a stable repo
// identity. SCP-like SSH URLs (git@host:org/repo) contain no credential token
// and are retained verbatim.
func NormalizeRepository(repository string) string {
	repository = strings.TrimSpace(repository)
	parsed, err := url.Parse(repository)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		if userHost, path, ok := strings.Cut(repository, ":"); ok {
			if _, host, hasUser := strings.Cut(userHost, "@"); hasUser && host != "" {
				return host + ":" + path
			}
		}
		return repository
	}
	parsed.User = nil
	return parsed.String()
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close() //nolint:errcheck
	return dir.Sync()
}

func syncFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0) //nolint:gosec // fixed operational path
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck
	return file.Sync()
}
