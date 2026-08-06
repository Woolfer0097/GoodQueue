package postgres

import (
	"context"
	"fmt"
	"testing"
)

func TestDemoUserRepositoryIntegrationListsFiveStableUsers(t *testing.T) {
	database := openIntegrationDatabase(t)
	users, err := NewDemoUserRepository(database).List(context.Background())
	if err != nil {
		t.Fatalf("list demo users: %v", err)
	}
	if len(users) != 5 {
		t.Fatalf("demo user count = %d, want 5", len(users))
	}
	for index, user := range users {
		expected := fmt.Sprintf("00000000-0000-4000-8000-%012d", index+1)
		if string(user.ExternalUserID) != expected {
			t.Errorf("user %d ID = %q, want %q", index, user.ExternalUserID, expected)
		}
		if user.DisplayName == "" {
			t.Errorf("user %d has an empty display name", index)
		}
	}
}
