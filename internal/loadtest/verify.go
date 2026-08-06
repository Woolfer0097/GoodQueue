package loadtest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

const detailLimit = 20

type VerificationResult struct {
	RunID       string              `json:"run_id"`
	Passed      bool                `json:"passed"`
	GeneratedAt time.Time           `json:"generated_at"`
	Counts      VerificationCounts  `json:"counts"`
	Checks      []VerificationCheck `json:"checks"`
}

type VerificationCounts struct {
	Users                 int `json:"users"`
	Products              int `json:"products"`
	Attempts              int `json:"attempts"`
	Waiting               int `json:"waiting"`
	Invited               int `json:"invited"`
	Checkout              int `json:"checkout"`
	Terminal              int `json:"terminal"`
	AllowedPairs          int `json:"allowed_pairs"`
	Purchased             int `json:"purchased"`
	Cancelled             int `json:"cancelled"`
	CheckoutExpired       int `json:"checkout_expired"`
	QueueRejected         int `json:"queue_rejected"`
	SoldOut               int `json:"sold_out"`
	Unresolved            int `json:"unresolved"`
	PaymentAccepted       int `json:"payment_accepted"`
	PaymentRejected       int `json:"payment_rejected"`
	PaymentTechnicalError int `json:"payment_technical_error"`
}

type VerificationCheck struct {
	Name       string   `json:"name"`
	Passed     bool     `json:"passed"`
	Violations []string `json:"violations,omitempty"`
	Note       string   `json:"note,omitempty"`
}

type productSnapshot struct {
	ID                string
	Title             string
	AllocatableStock  int32
	Reserved          int32
	NextQueueSequence int64
}

type attemptSnapshot struct {
	ID                 string
	ProductID          string
	QueueSequence      int64
	ExternalUserID     string
	IdempotencyKey     string
	State              string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	InvitedAt          *time.Time
	InvitationDeadline *time.Time
	CheckoutStartedAt  *time.Time
	CheckoutDeadline   *time.Time
	TerminalAt         *time.Time
	PurchasedAt        *time.Time
	AcceptedProvider   *string
	AcceptedReference  *string
	TerminalReason     *string
}

func Verify(ctx context.Context, connection *pgx.Conn, config Config, data Data) (VerificationResult, error) {
	if data.RunID != config.RunID {
		return VerificationResult{}, fmt.Errorf("data.json run_id %q does not match LOADTEST_RUN_ID %q", data.RunID, config.RunID)
	}
	prefix, err := RunPrefix(config.RunID)
	if err != nil {
		return VerificationResult{}, err
	}
	products, err := readProducts(ctx, connection, prefix)
	if err != nil {
		return VerificationResult{}, err
	}
	attempts, err := readAttempts(ctx, connection, prefix)
	if err != nil {
		return VerificationResult{}, err
	}
	users, err := readUserIDs(ctx, connection, prefix)
	if err != nil {
		return VerificationResult{}, err
	}
	result := Evaluate(config.RunID, data, users, products, attempts)
	if config.Scenario != ScenarioPurchaseOutcomes {
		outcomes := evaluateQueueScenario(data, attempts)
		result.Counts.Purchased = outcomes.Counts.Purchased
		result.Counts.Cancelled = outcomes.Counts.Cancelled
		result.Counts.CheckoutExpired = outcomes.Counts.CheckoutExpired
		result.Counts.QueueRejected = outcomes.Counts.QueueRejected
		result.Counts.SoldOut = outcomes.Counts.SoldOut
		result.Counts.Unresolved = outcomes.Counts.Unresolved
		if err := persistOutcomeEvaluation(ctx, connection, config.RunID, result.Passed, outcomes); err != nil {
			return VerificationResult{}, err
		}
		return result, nil
	}
	payments, err := readPayments(ctx, connection, prefix)
	if err != nil {
		return VerificationResult{}, err
	}
	eventPath := filepath.Join(config.ResultsDir, config.RunID, "k6-events.log")
	events, err := readK6OutcomeEvents(eventPath, config.RunID)
	if err != nil {
		return VerificationResult{}, err
	}
	outcomes := evaluatePurchaseOutcomes(data, products, attempts, payments, events)
	result.Counts.Purchased = outcomes.Counts.Purchased
	result.Counts.Cancelled = outcomes.Counts.Cancelled
	result.Counts.CheckoutExpired = outcomes.Counts.CheckoutExpired
	result.Counts.QueueRejected = outcomes.Counts.QueueRejected
	result.Counts.SoldOut = outcomes.Counts.SoldOut
	result.Counts.Unresolved = outcomes.Counts.Unresolved
	result.Counts.PaymentAccepted = outcomes.Counts.PaymentAccepted
	result.Counts.PaymentRejected = outcomes.Counts.PaymentRejected
	result.Counts.PaymentTechnicalError = outcomes.Counts.PaymentTechnicalError
	result.Checks = append(result.Checks, outcomes.Checks...)
	for _, check := range outcomes.Checks {
		if !check.Passed {
			result.Passed = false
		}
	}
	if err := persistOutcomeEvaluation(ctx, connection, config.RunID, result.Passed, outcomes); err != nil {
		return VerificationResult{}, err
	}
	return result, nil
}

func Evaluate(
	runID string,
	data Data,
	userIDs map[string]struct{},
	products []productSnapshot,
	attempts []attemptSnapshot,
) VerificationResult {
	result := VerificationResult{RunID: runID, Passed: true, GeneratedAt: time.Now().UTC()}
	result.Counts = VerificationCounts{
		Users: len(userIDs), Products: len(products), Attempts: len(attempts),
		AllowedPairs: allowedPairCount(data),
	}
	for _, attempt := range attempts {
		switch attempt.State {
		case "waiting":
			result.Counts.Waiting++
		case "invited":
			result.Counts.Invited++
		case "checkout":
			result.Counts.Checkout++
		default:
			result.Counts.Terminal++
		}
	}

	checks := []VerificationCheck{
		checkSeedRecords(data, userIDs, products),
		checkInventory(products),
		checkActiveAttempts(attempts),
		checkSequences(products, attempts),
		checkFIFO(attempts),
		checkAllowedAttempts(data, attempts),
		checkIdempotency(attempts),
		checkWaitingPositions(attempts),
		checkStateTimestamps(attempts),
		checkReferences(userIDs, products, attempts),
	}
	result.Checks = checks
	for _, check := range checks {
		if !check.Passed {
			result.Passed = false
		}
	}
	return result
}

func WriteVerification(path string, result VerificationResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create verifier result directory: %w", err)
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal verifier result: %w", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("write verifier result: %w", err)
	}
	return nil
}

func readProducts(ctx context.Context, connection *pgx.Conn, prefix string) ([]productSnapshot, error) {
	rows, err := connection.Query(ctx, `
		SELECT id::text, title, allocatable_stock, reserved, next_queue_sequence
		FROM products WHERE left(title, char_length($1)) = $1 ORDER BY id`, prefix)
	if err != nil {
		return nil, fmt.Errorf("read load-test products: %w", err)
	}
	products, err := pgx.CollectRows(rows, pgx.RowToStructByPos[productSnapshot])
	if err != nil {
		return nil, fmt.Errorf("collect load-test products: %w", err)
	}
	return products, nil
}

func readAttempts(ctx context.Context, connection *pgx.Conn, prefix string) ([]attemptSnapshot, error) {
	rows, err := connection.Query(ctx, `
		SELECT qa.id::text, qa.product_id::text, qa.queue_sequence, qa.external_user_id,
		       qa.idempotency_key, qa.state, qa.created_at, qa.updated_at, qa.invited_at,
		       qa.invitation_deadline, qa.checkout_started_at, qa.checkout_deadline,
		       qa.terminal_at, qa.purchased_at, qa.accepted_payment_provider,
		       qa.accepted_payment_reference, qa.terminal_reason
		FROM queue_attempts qa
		JOIN products p ON p.id = qa.product_id
		WHERE left(p.title, char_length($1)) = $1
		ORDER BY qa.product_id, qa.queue_sequence`, prefix)
	if err != nil {
		return nil, fmt.Errorf("read load-test attempts: %w", err)
	}
	attempts, err := pgx.CollectRows(rows, pgx.RowToStructByPos[attemptSnapshot])
	if err != nil {
		return nil, fmt.Errorf("collect load-test attempts: %w", err)
	}
	return attempts, nil
}

func readUserIDs(ctx context.Context, connection *pgx.Conn, prefix string) (map[string]struct{}, error) {
	rows, err := connection.Query(ctx, `
		SELECT external_user_id::text FROM users
		WHERE left(name, char_length($1)) = $1`, prefix)
	if err != nil {
		return nil, fmt.Errorf("read load-test users: %w", err)
	}
	defer rows.Close()
	users := make(map[string]struct{})
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("scan load-test user: %w", err)
		}
		users[userID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate load-test users: %w", err)
	}
	return users, nil
}

func checkSeedRecords(data Data, users map[string]struct{}, products []productSnapshot) VerificationCheck {
	violations := make([]string, 0)
	actualProducts := make(map[string]struct{}, len(products))
	for _, product := range products {
		actualProducts[product.ID] = struct{}{}
	}
	if len(users) != len(data.Users) {
		violations = append(violations, fmt.Sprintf("users=%d, expected=%d", len(users), len(data.Users)))
	}
	if len(products) != len(data.Products) {
		violations = append(violations, fmt.Sprintf("products=%d, expected=%d", len(products), len(data.Products)))
	}
	for _, user := range data.Users {
		if _, exists := users[user.ID]; !exists {
			violations = appendViolation(violations, "missing user "+user.ID)
		}
	}
	for _, product := range data.Products {
		if _, exists := actualProducts[product.ID]; !exists {
			violations = appendViolation(violations, "missing product "+product.ID)
		}
	}
	return makeCheck("seed_records_match_data_file", violations, "run-scoped users and products match data.json")
}

func checkInventory(products []productSnapshot) VerificationCheck {
	violations := make([]string, 0)
	for _, product := range products {
		if product.AllocatableStock < 0 {
			violations = appendViolation(violations, product.ID+": negative allocatable_stock")
		}
		if product.Reserved < 0 {
			violations = appendViolation(violations, product.ID+": negative reserved")
		}
		if product.Reserved > product.AllocatableStock {
			violations = appendViolation(violations, product.ID+": reserved exceeds allocatable_stock")
		}
	}
	return makeCheck("inventory_bounds", violations, "stock >= 0, reserved >= 0, reserved <= allocatable_stock")
}

func checkActiveAttempts(attempts []attemptSnapshot) VerificationCheck {
	counts := make(map[string]int)
	violations := make([]string, 0)
	for _, attempt := range attempts {
		if isActiveState(attempt.State) {
			key := attempt.ExternalUserID + "/" + attempt.ProductID
			counts[key]++
			if counts[key] > 1 {
				violations = appendViolation(violations, "multiple active attempts for "+key)
			}
		}
	}
	return makeCheck("one_active_attempt_per_user_product", violations, "active states are waiting, invited, and checkout")
}

func checkSequences(products []productSnapshot, attempts []attemptSnapshot) VerificationCheck {
	seen := make(map[string]struct{})
	maximum := make(map[string]int64)
	count := make(map[string]int64)
	violations := make([]string, 0)
	for _, attempt := range attempts {
		key := fmt.Sprintf("%s/%d", attempt.ProductID, attempt.QueueSequence)
		if _, exists := seen[key]; exists {
			violations = appendViolation(violations, "duplicate queue_sequence "+key)
		}
		seen[key] = struct{}{}
		count[attempt.ProductID]++
		maximum[attempt.ProductID] = max(maximum[attempt.ProductID], attempt.QueueSequence)
	}
	for _, product := range products {
		if maximum[product.ID] != count[product.ID] {
			violations = appendViolation(violations, fmt.Sprintf("%s: sequence max=%d count=%d", product.ID, maximum[product.ID], count[product.ID]))
		}
		if product.NextQueueSequence != maximum[product.ID]+1 {
			violations = appendViolation(violations, fmt.Sprintf("%s: next_queue_sequence=%d expected=%d", product.ID, product.NextQueueSequence, maximum[product.ID]+1))
		}
	}
	return makeCheck("queue_sequence_unique_and_contiguous", violations, "each run starts at sequence 1 and does not delete attempts")
}

func checkFIFO(attempts []attemptSnapshot) VerificationCheck {
	byProduct := attemptsByProduct(attempts)
	violations := make([]string, 0)
	for productID, productAttempts := range byProduct {
		sort.Slice(productAttempts, func(left, right int) bool {
			return productAttempts[left].QueueSequence < productAttempts[right].QueueSequence
		})
		for index := 1; index < len(productAttempts); index++ {
			if productAttempts[index].CreatedAt.Before(productAttempts[index-1].CreatedAt) {
				violations = appendViolation(violations, productID+": creation order contradicts queue_sequence")
			}
		}
	}
	return makeCheck("fifo_sequence_order", violations, "creation timestamps are monotonic in queue_sequence order")
}

func checkAllowedAttempts(data Data, attempts []attemptSnapshot) VerificationCheck {
	allowed := make(map[string]struct{})
	for _, user := range data.Users {
		for _, assignment := range user.Assignments {
			allowed[user.ID+"/"+assignment.ProductID] = struct{}{}
		}
	}
	violations := make([]string, 0)
	for _, attempt := range attempts {
		key := attempt.ExternalUserID + "/" + attempt.ProductID
		if _, exists := allowed[key]; !exists {
			violations = appendViolation(violations, "attempt outside generated assignments: "+key)
		}
	}
	if len(attempts) > len(allowed) {
		violations = appendViolation(violations, fmt.Sprintf("attempts=%d exceed unique allowed pairs=%d", len(attempts), len(allowed)))
	}
	return makeCheck("attempts_within_unique_user_product_links", violations, "expected domain rejections may make the count lower")
}

func checkIdempotency(attempts []attemptSnapshot) VerificationCheck {
	seen := make(map[string]string)
	violations := make([]string, 0)
	for _, attempt := range attempts {
		key := attempt.ExternalUserID + "/" + attempt.ProductID + "/" + attempt.IdempotencyKey
		if previous, exists := seen[key]; exists && previous != attempt.ID {
			violations = appendViolation(violations, "idempotency tuple maps to multiple attempts: "+key)
		}
		seen[key] = attempt.ID
	}
	return makeCheck("idempotent_join_single_attempt", violations, "same user, product, and Idempotency-Key creates at most one attempt")
}

func checkWaitingPositions(attempts []attemptSnapshot) VerificationCheck {
	byProduct := attemptsByProduct(attempts)
	violations := make([]string, 0)
	for productID, productAttempts := range byProduct {
		waiting := make([]attemptSnapshot, 0)
		for _, attempt := range productAttempts {
			if attempt.State == "waiting" {
				waiting = append(waiting, attempt)
			}
		}
		sort.Slice(waiting, func(left, right int) bool { return waiting[left].QueueSequence < waiting[right].QueueSequence })
		for index := 1; index < len(waiting); index++ {
			if waiting[index].QueueSequence <= waiting[index-1].QueueSequence {
				violations = appendViolation(violations, productID+": invalid waiting position order")
			}
		}
	}
	return makeCheck("waiting_position_and_total", violations, "position is lower-sequence waiting count + 1; total_waiting is the per-product waiting count")
}

func checkStateTimestamps(attempts []attemptSnapshot) VerificationCheck {
	violations := make([]string, 0)
	for _, attempt := range attempts {
		valid := !attempt.UpdatedAt.Before(attempt.CreatedAt)
		switch attempt.State {
		case "waiting":
			valid = valid && attempt.InvitedAt == nil && attempt.CheckoutStartedAt == nil && attempt.TerminalAt == nil && attempt.PurchasedAt == nil
		case "invited":
			valid = valid && validPair(attempt.InvitedAt, attempt.InvitationDeadline) && attempt.CheckoutStartedAt == nil && attempt.TerminalAt == nil
		case "checkout":
			valid = valid && validPair(attempt.InvitedAt, attempt.InvitationDeadline) && validPair(attempt.CheckoutStartedAt, attempt.CheckoutDeadline) && attempt.TerminalAt == nil
		case "purchased":
			valid = valid && attempt.InvitedAt != nil && attempt.CheckoutStartedAt != nil && attempt.PurchasedAt != nil && attempt.TerminalAt != nil && attempt.PurchasedAt.Equal(*attempt.TerminalAt) && attempt.AcceptedProvider != nil && attempt.AcceptedReference != nil
		case "invite_expired":
			valid = valid && attempt.InvitedAt != nil && attempt.InvitationDeadline != nil && attempt.CheckoutStartedAt == nil && deadlineReached(attempt.TerminalAt, attempt.InvitationDeadline)
		case "checkout_expired":
			valid = valid && attempt.InvitedAt != nil && attempt.CheckoutStartedAt != nil && deadlineReached(attempt.TerminalAt, attempt.CheckoutDeadline)
		case "payment_failed":
			valid = valid && attempt.InvitedAt != nil && attempt.CheckoutStartedAt != nil && attempt.TerminalAt != nil
		case "cancelled":
			valid = valid && attempt.TerminalAt != nil && attempt.PurchasedAt == nil
		case "sold_out":
			valid = valid && attempt.InvitedAt == nil && attempt.CheckoutStartedAt == nil && attempt.TerminalAt != nil
		default:
			valid = false
		}
		if isActiveState(attempt.State) {
			valid = valid && attempt.TerminalReason == nil
		} else {
			valid = valid && attempt.TerminalReason != nil
		}
		if attempt.State != "purchased" {
			valid = valid && attempt.AcceptedProvider == nil && attempt.AcceptedReference == nil
		}
		if !valid {
			violations = appendViolation(violations, attempt.ID+": impossible state/timestamp combination for "+attempt.State)
		}
	}
	return makeCheck("state_timestamp_combinations", violations, "matches the queue_attempts domain constraints")
}

func checkReferences(users map[string]struct{}, products []productSnapshot, attempts []attemptSnapshot) VerificationCheck {
	productIDs := make(map[string]struct{}, len(products))
	for _, product := range products {
		productIDs[product.ID] = struct{}{}
	}
	violations := make([]string, 0)
	for _, attempt := range attempts {
		if _, exists := productIDs[attempt.ProductID]; !exists {
			violations = appendViolation(violations, attempt.ID+": missing run-scoped product")
		}
		if _, exists := users[attempt.ExternalUserID]; !exists {
			violations = appendViolation(violations, attempt.ID+": external_user_id has no run-scoped user")
		}
	}
	return makeCheck("references_resolve", violations, "product FK and external user references resolve inside the run")
}

func attemptsByProduct(attempts []attemptSnapshot) map[string][]attemptSnapshot {
	result := make(map[string][]attemptSnapshot)
	for _, attempt := range attempts {
		result[attempt.ProductID] = append(result[attempt.ProductID], attempt)
	}
	return result
}

func allowedPairCount(data Data) int {
	total := 0
	for _, user := range data.Users {
		total += len(user.Assignments)
	}
	return total
}

func makeCheck(name string, violations []string, note string) VerificationCheck {
	return VerificationCheck{Name: name, Passed: len(violations) == 0, Violations: violations, Note: note}
}

func appendViolation(violations []string, violation string) []string {
	if len(violations) < detailLimit {
		return append(violations, violation)
	}
	return violations
}

func isActiveState(state string) bool {
	return state == "waiting" || state == "invited" || state == "checkout"
}

func validPair(start, deadline *time.Time) bool {
	return start != nil && deadline != nil && deadline.After(*start)
}

func deadlineReached(terminal, deadline *time.Time) bool {
	return terminal != nil && deadline != nil && !terminal.Before(*deadline)
}
