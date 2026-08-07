package loadtest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

type Data struct {
	RunID           string          `json:"run_id"`
	RandomSeed      int64           `json:"random_seed"`
	EffectiveConfig EffectiveConfig `json:"effective_config"`
	Users           []User          `json:"users"`
	Products        []Product       `json:"products"`
}

type User struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Assignments []Assignment `json:"assignments"`
}

type Product struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	InitialStock  int    `json:"initial_stock"`
	QueueCapacity int    `json:"queue_capacity"`
	Group         string `json:"group"`
}

type Assignment struct {
	ProductID        string `json:"product_id"`
	ProductGroup     string `json:"product_group"`
	IdempotencyKey   string `json:"idempotency_key"`
	DuplicateJoin    bool   `json:"duplicate_join"`
	PlannedOutcome   string `json:"planned_outcome,omitempty"`
	PaymentEventID   string `json:"payment_event_id,omitempty"`
	PaymentReference string `json:"payment_reference,omitempty"`
}

func GenerateData(config Config) (Data, error) {
	if err := config.Validate(); err != nil {
		return Data{}, err
	}
	prefix, err := RunPrefix(config.RunID)
	if err != nil {
		return Data{}, err
	}
	random := rand.New(rand.NewSource(config.RandomSeed)) //nolint:gosec // Reproducibility is required, not cryptographic randomness.
	products := make([]Product, config.Products)
	for index := range products {
		products[index] = Product{
			ID:            deterministicUUID(config.RunID, "product", index+1),
			Title:         fmt.Sprintf("%sProduct-%03d", prefix, index+1),
			InitialStock:  config.MinStock + random.Intn(config.MaxStock-config.MinStock+1),
			QueueCapacity: config.QueueCapacity,
			Group:         productGroup(index, config.Products),
		}
	}

	var users []User
	for attempt := 0; attempt < 1000; attempt++ {
		users, err = generateUsers(config, products, random, prefix)
		if err == nil {
			break
		}
	}
	if err != nil {
		return Data{}, fmt.Errorf("allocate user-product assignments after deterministic retries: %w", err)
	}
	return Data{
		RunID: config.RunID, RandomSeed: config.RandomSeed, EffectiveConfig: config.Effective(),
		Users: users, Products: products,
	}, nil
}

func generateUsers(config Config, products []Product, random *rand.Rand, prefix string) ([]User, error) {
	assignmentCounts := make([]int, len(products))
	users := make([]User, config.Users)
	for userIndex := range users {
		user := User{
			ID:          deterministicUUID(config.RunID, "user", userIndex+1),
			Name:        fmt.Sprintf("%sUser-%04d", prefix, userIndex+1),
			Assignments: make([]Assignment, 0, config.ProductsPerUser),
		}
		selected := make(map[int]struct{}, config.ProductsPerUser)
		for assignmentIndex := 0; assignmentIndex < config.ProductsPerUser; assignmentIndex++ {
			productIndex, selectErr := selectProduct(random, products, selected, assignmentCounts, config.QueueCapacity)
			if selectErr != nil {
				return nil, selectErr
			}
			selected[productIndex] = struct{}{}
			assignmentCounts[productIndex]++
			key := idempotencyKey(config.RunID, user.ID, products[productIndex].ID)
			assignment := Assignment{
				ProductID: products[productIndex].ID, ProductGroup: products[productIndex].Group,
				IdempotencyKey: key, DuplicateJoin: random.Intn(100) < config.DuplicateJoinPercent,
			}
			if config.Scenario == ScenarioPurchaseOutcomes {
				assignment.PlannedOutcome = []string{"purchase", "cancel", "ttl"}[random.Intn(3)]
				if assignment.PlannedOutcome == "purchase" {
					assignment.PaymentEventID = paymentIdentifier("event", config.RunID, user.ID, assignment.ProductID)
					assignment.PaymentReference = paymentIdentifier("reference", config.RunID, user.ID, assignment.ProductID)
				}
			}
			user.Assignments = append(user.Assignments, assignment)
		}
		users[userIndex] = user
	}
	return users, nil
}

func WriteData(path string, data Data) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create load-test data directory: %w", err)
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal load-test data: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".data-*.json")
	if err != nil {
		return fmt.Errorf("create temporary load-test data: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err = temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(append(encoded, '\n'))
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write load-test data: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("publish load-test data: %w", err)
	}
	return nil
}

func ReadData(path string) (Data, error) {
	encoded, err := os.ReadFile(path) //nolint:gosec // The operator explicitly configures the fixture path.
	if err != nil {
		return Data{}, fmt.Errorf("read load-test data: %w", err)
	}
	var data Data
	if err := json.Unmarshal(encoded, &data); err != nil {
		return Data{}, fmt.Errorf("decode load-test data: %w", err)
	}
	return data, nil
}

func deterministicUUID(runID, kind string, index int) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("goodqueue-loadtest:%s:%s:%d", runID, kind, index))).String()
}

func idempotencyKey(runID, userID, productID string) string {
	digest := sha256.Sum256([]byte(runID + ":" + userID + ":" + productID))
	return "lt-" + hex.EncodeToString(digest[:16])
}

func paymentIdentifier(kind, runID, userID, productID string) string {
	digest := sha256.Sum256([]byte(kind + ":" + runID + ":" + userID + ":" + productID))
	return "lt-" + kind + "-" + hex.EncodeToString(digest[:16])
}

func productGroup(index, total int) string {
	hotCount := max(1, (total+9)/10)
	mediumCount := (total*3 + 9) / 10
	if hotCount+mediumCount > total {
		mediumCount = total - hotCount
	}
	switch {
	case index < hotCount:
		return "hot"
	case index < hotCount+mediumCount:
		return "medium"
	default:
		return "normal"
	}
}

func selectProduct(
	random *rand.Rand,
	products []Product,
	selected map[int]struct{},
	assignmentCounts []int,
	capacity int,
) (int, error) {
	roll := random.Intn(100)
	preferredGroup := "normal"
	if roll < 60 {
		preferredGroup = "hot"
	} else if roll < 90 {
		preferredGroup = "medium"
	}
	candidates := productCandidates(products, selected, assignmentCounts, capacity, preferredGroup)
	if len(candidates) == 0 {
		candidates = productCandidates(products, selected, assignmentCounts, capacity, "")
	}
	if len(candidates) == 0 {
		return 0, fmt.Errorf("cannot allocate distinct products within LOADTEST_QUEUE_CAPACITY")
	}
	return candidates[random.Intn(len(candidates))], nil
}

func productCandidates(
	products []Product,
	selected map[int]struct{},
	assignmentCounts []int,
	capacity int,
	group string,
) []int {
	candidates := make([]int, 0, len(products))
	for index, product := range products {
		if _, exists := selected[index]; exists || assignmentCounts[index] >= capacity {
			continue
		}
		if group == "" || product.Group == group {
			candidates = append(candidates, index)
		}
	}
	return candidates
}
