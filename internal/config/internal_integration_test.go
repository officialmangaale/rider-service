package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"
)

func TestInternalIntegrationIssues(t *testing.T) {
	complete := &Config{RestaurantServiceBaseURL: "https://restaurant.example", InternalServiceToken: "secret"}
	if got := complete.InternalIntegrationIssues(); len(got) != 0 {
		t.Fatalf("complete config reported issues: %v", got)
	}

	blank := &Config{RestaurantServiceBaseURL: "  ", InternalServiceToken: ""}
	got := blank.InternalIntegrationIssues()
	if len(got) != 2 {
		t.Fatalf("expected both settings reported, got %v", got)
	}
	for _, issue := range got {
		if issue == "" {
			t.Fatal("empty issue text")
		}
	}
}

// The incident this guards against: an env file that defines a key with a real
// value and then again, empty, further down. The later line wins, so the
// service starts with no restaurant-service URL or token. This test pins that
// behaviour so the committed .env can be checked for it.
func TestLaterEmptyDuplicateOverridesEarlierValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "RESTAURANT_SERVICE_INTERNAL_BASE_URL=https://restaurant.example\n" +
		"INTERNAL_SERVICE_TOKEN=abc\n" +
		"RESTAURANT_SERVICE_INTERNAL_BASE_URL=\n" +
		"INTERNAL_SERVICE_TOKEN=\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := godotenv.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if m["RESTAURANT_SERVICE_INTERNAL_BASE_URL"] != "" || m["INTERNAL_SERVICE_TOKEN"] != "" {
		t.Fatal("expected the later empty duplicates to win; the loader's behaviour changed")
	}
}

// The committed .env must not repeat the restaurant-service settings. Reads
// the file as the loader would and fails if either key resolves to empty.
func TestRepositoryEnvKeepsRestaurantServiceSettings(t *testing.T) {
	path := filepath.Join("..", "..", ".env")
	if _, err := os.Stat(path); err != nil {
		t.Skip("no .env in this checkout")
	}
	m, err := godotenv.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"RESTAURANT_SERVICE_INTERNAL_BASE_URL", "INTERNAL_SERVICE_TOKEN"} {
		if m[key] == "" {
			t.Errorf("%s resolves to empty in .env (an empty duplicate later in the file overrides the real value)", key)
		}
	}
}
