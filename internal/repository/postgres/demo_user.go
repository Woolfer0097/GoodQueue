package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

type DemoUserRepository struct{ db *sql.DB }

func NewDemoUserRepository(db *sql.DB) *DemoUserRepository {
	return &DemoUserRepository{db: db}
}

func (repository *DemoUserRepository) List(ctx context.Context) ([]domain.DemoUser, error) {
	rows, err := repository.db.QueryContext(ctx, `
		SELECT external_user_id::text, name
		FROM users
		ORDER BY external_user_id`)
	if err != nil {
		return nil, fmt.Errorf("query demo users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	users := make([]domain.DemoUser, 0)
	for rows.Next() {
		var rawID string
		var user domain.DemoUser
		if err := rows.Scan(&rawID, &user.DisplayName); err != nil {
			return nil, fmt.Errorf("scan demo user: %w", err)
		}
		userID, err := domain.ParseExternalUserID(rawID)
		if err != nil {
			return nil, fmt.Errorf("parse stored demo user identity: %w", err)
		}
		user.ExternalUserID = userID
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate demo users: %w", err)
	}
	return users, nil
}
