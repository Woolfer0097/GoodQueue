package loadtest

import (
	"reflect"
	"testing"
)

func TestGenerateDataIsDeterministicAndDistinctPerUser(t *testing.T) {
	t.Parallel()
	config, err := LoadConfigFrom(func(key string) (string, bool) {
		values := map[string]string{
			"LOADTEST_RUN_ID": "unit", "LOADTEST_USERS": "30", "LOADTEST_PRODUCTS": "10",
			"LOADTEST_PRODUCTS_PER_USER": "5", "LOADTEST_RANDOM_SEED": "123",
		}
		value, exists := values[key]
		return value, exists
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := GenerateData(config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateData(config)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("GenerateData() is not deterministic")
	}
	for _, user := range first.Users {
		seen := make(map[string]struct{})
		for _, assignment := range user.Assignments {
			if _, exists := seen[assignment.ProductID]; exists {
				t.Fatalf("user %s has duplicate product %s", user.ID, assignment.ProductID)
			}
			seen[assignment.ProductID] = struct{}{}
		}
	}
}

func TestGenerateDataProducesRecognizableRecordsAndSkew(t *testing.T) {
	t.Parallel()
	config, err := LoadConfigFrom(func(key string) (string, bool) {
		values := map[string]string{
			"LOADTEST_RUN_ID": "skew", "LOADTEST_USERS": "100", "LOADTEST_PRODUCTS": "20",
			"LOADTEST_PRODUCTS_PER_USER": "5", "LOADTEST_QUEUE_CAPACITY": "1000",
		}
		value, exists := values[key]
		return value, exists
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := GenerateData(config)
	if err != nil {
		t.Fatal(err)
	}
	groupCounts := map[string]int{}
	for _, user := range data.Users {
		if len(user.Name) < len("LT-skew-User-") || user.Name[:len("LT-skew-User-")] != "LT-skew-User-" {
			t.Fatalf("unrecognizable user name %q", user.Name)
		}
		for _, assignment := range user.Assignments {
			groupCounts[assignment.ProductGroup]++
		}
	}
	if groupCounts["hot"] <= groupCounts["normal"] {
		t.Fatalf("weighted distribution is not skewed toward hot products: %v", groupCounts)
	}
}

func TestGenerateDataPurchaseOutcomesAreDeterministic(t *testing.T) {
	t.Parallel()
	values := map[string]string{
		"LOADTEST_RUN_ID": "outcomes", "LOADTEST_SCENARIO": "purchase_outcomes",
		"LOADTEST_USERS": "30", "LOADTEST_PRODUCTS": "10", "LOADTEST_PRODUCTS_PER_USER": "5",
		"LOADTEST_RANDOM_SEED": "9876",
	}
	config, err := LoadConfigFrom(func(key string) (string, bool) {
		value, exists := values[key]
		return value, exists
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := GenerateData(config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateData(config)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("purchase outcome generation is not deterministic")
	}
	counts := map[string]int{}
	for _, user := range first.Users {
		for _, assignment := range user.Assignments {
			counts[assignment.PlannedOutcome]++
			if assignment.PlannedOutcome == "purchase" {
				if assignment.PaymentEventID == "" || assignment.PaymentReference == "" {
					t.Fatal("purchase assignment has no stable payment identifiers")
				}
			} else if assignment.PaymentEventID != "" || assignment.PaymentReference != "" {
				t.Fatalf("%s assignment unexpectedly has payment identifiers", assignment.PlannedOutcome)
			}
		}
	}
	for _, outcome := range []string{"purchase", "cancel", "ttl"} {
		if counts[outcome] == 0 {
			t.Fatalf("large deterministic fixture did not contain %s: %v", outcome, counts)
		}
	}
}
