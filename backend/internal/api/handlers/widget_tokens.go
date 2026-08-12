package handlers

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// getOrCreateWidgetToken returns this user's OBS alert-widget token,
// minting one if they don't have one yet. The table is named after Twitch
// (the first alert source built), but the token/widget itself is generic --
// donations broadcast through the same widget and the same OBS Browser
// Source rather than needing a second one.
func getOrCreateWidgetToken(ctx context.Context, db *pgxpool.Pool, userID int64) (string, error) {
	var token string
	err := db.QueryRow(ctx, `SELECT token FROM twitch_widget_tokens WHERE user_id = $1`, userID).Scan(&token)
	if err == nil {
		return token, nil
	}
	if err != pgx.ErrNoRows {
		return "", err
	}

	token, err = randomToken(24)
	if err != nil {
		return "", err
	}
	if _, err := db.Exec(ctx,
		`INSERT INTO twitch_widget_tokens (user_id, token) VALUES ($1, $2)`, userID, token,
	); err != nil {
		return "", err
	}
	return token, nil
}
