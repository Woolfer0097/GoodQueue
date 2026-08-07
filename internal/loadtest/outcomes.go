package loadtest

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
)

type paymentSnapshot struct {
	ID                 string
	Provider           string
	EventID            string
	AttemptID          string
	Outcome            string
	Status             string
	ResponseHTTPStatus *int16
	LastError          *string
}

type requestLogResult struct {
	UserID         string
	ProductID      string
	AttemptID      *string
	Operation      string
	HTTPStatus     *int16
	PaymentEventID *string
	PaymentInboxID *string
	FinalState     string
	ActualOutcome  string
	TechnicalError *string
}

type outcomeEvaluation struct {
	Counts VerificationCounts
	Checks []VerificationCheck
	Logs   []requestLogResult
}

type k6OutcomeEvent struct {
	RunID          string `json:"run_id"`
	ExternalUserID string `json:"external_user_id"`
	ProductID      string `json:"product_id"`
	AttemptID      string `json:"attempt_id"`
	Operation      string `json:"operation"`
	HTTPStatus     *int16 `json:"http_status"`
	PaymentEventID string `json:"payment_event_id"`
	FinalState     string `json:"final_state"`
	ActualOutcome  string `json:"actual_outcome"`
	TechnicalError string `json:"technical_error"`
}

func readK6OutcomeEvents(path, runID string) (map[string]k6OutcomeEvent, error) {
	file, err := os.Open(path) //nolint:gosec // The operator controls the run-scoped results path.
	if err != nil {
		return nil, fmt.Errorf("open k6 outcome log %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	const marker = "GOODQUEUE_OUTCOME "
	events := make(map[string]k6OutcomeEvent)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		index := strings.Index(line, marker)
		if index < 0 {
			continue
		}
		var event k6OutcomeEvent
		if err := json.Unmarshal([]byte(line[index+len(marker):]), &event); err != nil {
			return nil, fmt.Errorf("decode k6 outcome event: %w", err)
		}
		if event.RunID != runID {
			return nil, fmt.Errorf("k6 outcome event run_id %q does not match %q", event.RunID, runID)
		}
		key := event.ExternalUserID + "/" + event.ProductID
		if _, exists := events[key]; exists {
			return nil, fmt.Errorf("duplicate k6 outcome event for %s", key)
		}
		events[key] = event
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read k6 outcome log: %w", err)
	}
	return events, nil
}

func readPayments(ctx context.Context, connection *pgx.Conn, prefix string) ([]paymentSnapshot, error) {
	rows, err := connection.Query(ctx, `
		SELECT pi.id::text, pi.provider, pi.event_id, pi.attempt_id::text, pi.outcome, pi.status,
		       pi.response_http_status, pi.last_error
		FROM payment_inbox pi
		JOIN queue_attempts qa ON qa.id = pi.attempt_id
		JOIN products p ON p.id = qa.product_id
		WHERE left(p.title, char_length($1)) = $1
		ORDER BY pi.created_at, pi.id`, prefix)
	if err != nil {
		return nil, fmt.Errorf("read load-test payments: %w", err)
	}
	payments, err := pgx.CollectRows(rows, pgx.RowToStructByPos[paymentSnapshot])
	if err != nil {
		return nil, fmt.Errorf("collect load-test payments: %w", err)
	}
	return payments, nil
}

func evaluateQueueScenario(data Data, attempts []attemptSnapshot) outcomeEvaluation {
	attemptByPair := make(map[string]attemptSnapshot, len(attempts))
	for _, attempt := range attempts {
		attemptByPair[attempt.ExternalUserID+"/"+attempt.ProductID] = attempt
	}
	evaluation := outcomeEvaluation{}
	for _, user := range data.Users {
		for _, assignment := range user.Assignments {
			log := requestLogResult{
				UserID: user.ID, ProductID: assignment.ProductID,
				Operation: "GET /api/v1/products/{productID}/queue-entry",
			}
			attempt, exists := attemptByPair[user.ID+"/"+assignment.ProductID]
			if !exists {
				log.Operation = "POST /api/v1/products/{productID}/queue-entries"
				log.FinalState = "queue_rejected"
				log.ActualOutcome = "queue_rejected"
				evaluation.Counts.QueueRejected++
				evaluation.Logs = append(evaluation.Logs, log)
				continue
			}
			log.AttemptID = stringPointer(attempt.ID)
			log.FinalState = attempt.State
			log.ActualOutcome = actualOutcome(attempt.State)
			switch log.ActualOutcome {
			case "purchased":
				evaluation.Counts.Purchased++
			case "cancelled":
				evaluation.Counts.Cancelled++
			case "checkout_expired":
				evaluation.Counts.CheckoutExpired++
			case "sold_out":
				evaluation.Counts.SoldOut++
			default:
				evaluation.Counts.Unresolved++
			}
			evaluation.Logs = append(evaluation.Logs, log)
		}
	}
	return evaluation
}

func evaluatePurchaseOutcomes(
	data Data,
	products []productSnapshot,
	attempts []attemptSnapshot,
	payments []paymentSnapshot,
	events map[string]k6OutcomeEvent,
) outcomeEvaluation {
	attemptByPair := make(map[string]attemptSnapshot, len(attempts))
	for _, attempt := range attempts {
		attemptByPair[attempt.ExternalUserID+"/"+attempt.ProductID] = attempt
	}
	paymentByAttempt := make(map[string][]paymentSnapshot)
	for _, payment := range payments {
		paymentByAttempt[payment.AttemptID] = append(paymentByAttempt[payment.AttemptID], payment)
	}

	evaluation := outcomeEvaluation{}
	mappingViolations := make([]string, 0)
	paymentViolations := make([]string, 0)
	activeViolations := make([]string, 0)
	stockViolations := make([]string, 0)
	eventViolations := make([]string, 0)
	purchasedByProduct := make(map[string]int)
	allowedEvents := make(map[string]struct{}, allowedPairCount(data))

	for _, user := range data.Users {
		for _, assignment := range user.Assignments {
			pair := user.ID + "/" + assignment.ProductID
			allowedEvents[pair] = struct{}{}
			event, eventExists := events[pair]
			log := requestLogResult{
				UserID: user.ID, ProductID: assignment.ProductID,
				Operation: event.Operation, HTTPStatus: event.HTTPStatus,
				FinalState: event.FinalState, ActualOutcome: event.ActualOutcome,
			}
			if event.PaymentEventID != "" {
				log.PaymentEventID = stringPointer(event.PaymentEventID)
			}
			if event.TechnicalError != "" {
				log.TechnicalError = stringPointer(event.TechnicalError)
			}
			if !eventExists {
				eventViolations = appendViolation(eventViolations, "missing k6 outcome event for "+pair)
				log.Operation = "k6_outcome_missing"
				log.FinalState = "unresolved"
				log.ActualOutcome = "unresolved"
				message := "k6 outcome event is missing"
				log.TechnicalError = &message
			}

			attempt, attemptExists := attemptByPair[pair]
			if !attemptExists {
				switch log.ActualOutcome {
				case "queue_rejected":
					evaluation.Counts.QueueRejected++
				case "sold_out":
					evaluation.Counts.SoldOut++
				case "unresolved":
					evaluation.Counts.Unresolved++
					activeViolations = appendViolation(activeViolations, pair+": no attempt and unresolved outcome")
				default:
					evaluation.Counts.Unresolved++
					eventViolations = appendViolation(eventViolations, pair+": outcome without attempt is "+log.ActualOutcome)
					log.ActualOutcome = "unresolved"
				}
				evaluation.Logs = append(evaluation.Logs, log)
				continue
			}

			log.AttemptID = stringPointer(attempt.ID)
			actual := actualOutcome(attempt.State)
			if eventExists && event.AttemptID != "" && event.AttemptID != attempt.ID {
				eventViolations = appendViolation(eventViolations, pair+": k6 attempt_id does not match PostgreSQL")
			}
			if eventExists && event.ActualOutcome != actual {
				eventViolations = appendViolation(eventViolations, fmt.Sprintf(
					"%s: k6 actual=%s PostgreSQL actual=%s", pair, event.ActualOutcome, actual,
				))
			}
			log.FinalState = attempt.State
			log.ActualOutcome = actual
			attemptPayments := paymentByAttempt[attempt.ID]
			switch actual {
			case "purchased":
				evaluation.Counts.Purchased++
				purchasedByProduct[assignment.ProductID]++
				if log.Operation == "" {
					log.Operation = "POST /internal/v1/payment-events"
				}
			case "cancelled":
				evaluation.Counts.Cancelled++
				if log.Operation == "" {
					log.Operation = "DELETE /api/v1/products/{productID}/queue-entry"
				}
			case "checkout_expired":
				evaluation.Counts.CheckoutExpired++
				if log.Operation == "" {
					log.Operation = "wait_checkout_ttl"
				}
			case "sold_out":
				evaluation.Counts.SoldOut++
				if log.Operation == "" {
					log.Operation = "GET /api/v1/products/{productID}/queue-entry"
				}
			default:
				evaluation.Counts.Unresolved++
				if log.Operation == "" {
					log.Operation = "poll_until_terminal"
				}
				message := "attempt remained in state " + attempt.State
				log.TechnicalError = &message
				activeViolations = appendViolation(activeViolations, attempt.ID+": "+attempt.State)
			}

			if len(attemptPayments) > 0 {
				payment := attemptPayments[0]
				log.PaymentEventID = stringPointer(payment.EventID)
				log.PaymentInboxID = stringPointer(payment.ID)
				if log.HTTPStatus == nil {
					log.HTTPStatus = payment.ResponseHTTPStatus
				}
				if payment.LastError != nil {
					log.TechnicalError = payment.LastError
				}
			} else if assignment.PlannedOutcome == "purchase" && attempt.CheckoutStartedAt != nil {
				evaluation.Counts.PaymentTechnicalError++
				log.Operation = "POST /internal/v1/payment-events"
				message := "payment request produced no payment_inbox record"
				log.TechnicalError = &message
			}

			if attempt.CheckoutStartedAt != nil && actual != "unresolved" {
				expected := map[string]string{"purchase": "purchased", "cancel": "cancelled", "ttl": "checkout_expired"}[assignment.PlannedOutcome]
				if actual != expected {
					mappingViolations = appendViolation(mappingViolations, fmt.Sprintf(
						"%s/%s: planned=%s actual=%s", user.ID, assignment.ProductID, assignment.PlannedOutcome, actual,
					))
					message := fmt.Sprintf("planned outcome %s does not match %s", assignment.PlannedOutcome, actual)
					log.TechnicalError = &message
				}
			}

			if actual == "purchased" {
				if assignment.PlannedOutcome != "purchase" || len(attemptPayments) != 1 {
					paymentViolations = appendViolation(paymentViolations, fmt.Sprintf(
						"%s: purchased requires exactly one planned payment; planned=%s payments=%d",
						attempt.ID, assignment.PlannedOutcome, len(attemptPayments),
					))
				} else {
					payment := attemptPayments[0]
					if payment.Provider != "goodqueue-loadtest" || payment.EventID != assignment.PaymentEventID || payment.Outcome != "succeeded" ||
						payment.Status != "completed" || payment.ResponseHTTPStatus == nil || *payment.ResponseHTTPStatus < 200 || *payment.ResponseHTTPStatus >= 300 {
						paymentViolations = appendViolation(paymentViolations, attempt.ID+": payment_inbox event was not accepted and completed")
					}
				}
			} else if len(attemptPayments) != 0 {
				paymentViolations = appendViolation(paymentViolations, fmt.Sprintf(
					"%s: %s must not have payment events (found %d)", attempt.ID, actual, len(attemptPayments),
				))
			}
			evaluation.Logs = append(evaluation.Logs, log)
		}
	}
	for pair := range events {
		if _, exists := allowedEvents[pair]; !exists {
			eventViolations = appendViolation(eventViolations, "k6 outcome event is outside generated assignments: "+pair)
		}
	}

	for _, payment := range payments {
		switch {
		case payment.Status == "completed" && payment.ResponseHTTPStatus != nil && *payment.ResponseHTTPStatus >= 200 && *payment.ResponseHTTPStatus < 300:
			evaluation.Counts.PaymentAccepted++
		case payment.Status == "rejected" || (payment.ResponseHTTPStatus != nil && *payment.ResponseHTTPStatus >= 400 && *payment.ResponseHTTPStatus < 500):
			evaluation.Counts.PaymentRejected++
		default:
			evaluation.Counts.PaymentTechnicalError++
		}
	}

	initialStock := make(map[string]int, len(data.Products))
	for _, product := range data.Products {
		initialStock[product.ID] = product.InitialStock
	}
	for _, product := range products {
		if product.Reserved != 0 {
			stockViolations = appendViolation(stockViolations, fmt.Sprintf("%s: reserved=%d expected=0", product.ID, product.Reserved))
		}
		expected := initialStock[product.ID] - purchasedByProduct[product.ID]
		if int(product.AllocatableStock) != expected {
			stockViolations = appendViolation(stockViolations, fmt.Sprintf(
				"%s: allocatable_stock=%d expected=%d (initial=%d purchased=%d)",
				product.ID, product.AllocatableStock, expected, initialStock[product.ID], purchasedByProduct[product.ID],
			))
		}
	}

	evaluation.Checks = []VerificationCheck{
		makeCheck("k6_outcome_events_complete", eventViolations, "one run-scoped k6 event exists for every user-product pair and matches PostgreSQL"),
		makeCheck("planned_purchase_outcomes_match", mappingViolations, "purchase -> purchased, cancel -> cancelled, ttl -> checkout_expired after checkout starts"),
		makeCheck("payment_events_match_outcomes", paymentViolations, "purchases have one accepted payment_inbox event; cancel and TTL have none"),
		makeCheck("purchase_scenario_has_no_unresolved_attempts", activeViolations, "no waiting, invited, checkout, or unexpected terminal attempts remain"),
		makeCheck("purchase_inventory_released", stockViolations, "reserved is released and stock decreases only for purchased attempts"),
	}
	return evaluation
}

func actualOutcome(state string) string {
	switch state {
	case "purchased", "cancelled", "checkout_expired", "sold_out":
		return state
	default:
		return "unresolved"
	}
}

func persistOutcomeEvaluation(
	ctx context.Context,
	connection *pgx.Conn,
	runID string,
	passed bool,
	evaluation outcomeEvaluation,
) error {
	transaction, err := connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin load-test outcome persistence: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	for _, log := range evaluation.Logs {
		commandTag, execErr := transaction.Exec(ctx, `
			UPDATE loadtest.request_logs SET
				attempt_id = $4::uuid,
				operation = $5,
				http_status = $6,
				payment_event_id = COALESCE($7, payment_event_id),
				payment_inbox_id = $8::uuid,
				final_state = $9,
				actual_outcome = $10,
				technical_error = $11,
				updated_at = clock_timestamp(),
				completed_at = clock_timestamp()
			WHERE run_id = $1 AND external_user_id = $2::uuid AND product_id = $3::uuid`,
			runID, log.UserID, log.ProductID, log.AttemptID, log.Operation, log.HTTPStatus,
			log.PaymentEventID, log.PaymentInboxID, log.FinalState, log.ActualOutcome, log.TechnicalError,
		)
		if execErr != nil {
			return fmt.Errorf("update loadtest.request_logs: %w", execErr)
		}
		if commandTag.RowsAffected() != 1 {
			return fmt.Errorf("update loadtest.request_logs: pair %s/%s is missing", log.UserID, log.ProductID)
		}
	}

	status := "failed"
	if passed {
		status = "completed"
	}
	counts := evaluation.Counts
	commandTag, err := transaction.Exec(ctx, `
		UPDATE loadtest.runs SET
			status = $2,
			actual_purchased = $3,
			actual_cancelled = $4,
			actual_checkout_expired = $5,
			actual_queue_rejected = $6,
			actual_sold_out = $7,
			actual_unresolved = $8,
			payment_accepted = $9,
			payment_rejected = $10,
			payment_technical_error = $11,
			verification_passed = $12,
			completed_at = clock_timestamp()
		WHERE run_id = $1`,
		runID, status, counts.Purchased, counts.Cancelled, counts.CheckoutExpired,
		counts.QueueRejected, counts.SoldOut, counts.Unresolved, counts.PaymentAccepted,
		counts.PaymentRejected, counts.PaymentTechnicalError, passed,
	)
	if err != nil {
		return fmt.Errorf("update loadtest.runs: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return fmt.Errorf("update loadtest.runs: run %q is missing", runID)
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit load-test outcome persistence: %w", err)
	}
	return nil
}

func stringPointer(value string) *string { return &value }
