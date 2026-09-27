//go:build integration

package repository

import (
	"context"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration241ReasoningEffortMultipliers(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	// Recreate the actual slim pre-241 shape: neither pricing table had the
	// reasoning_effort_multipliers column, and there was no legacy max column.
	_, err := tx.ExecContext(ctx, `ALTER TABLE channel_model_pricing DROP COLUMN IF EXISTS reasoning_effort_multipliers`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `ALTER TABLE channel_account_stats_model_pricing DROP COLUMN IF EXISTS reasoning_effort_multipliers`)
	require.NoError(t, err)

	var channelID, pricingID int64
	require.NoError(t, tx.QueryRowContext(ctx,
		`INSERT INTO channels (name) VALUES ('migration-reasoning-multipliers') RETURNING id`,
	).Scan(&channelID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO channel_model_pricing (channel_id, models)
VALUES ($1, '["claude-opus-5-5"]') RETURNING id`, channelID).Scan(&pricingID))

	var ruleID int64
	require.NoError(t, tx.QueryRowContext(ctx,
		`INSERT INTO channel_account_stats_pricing_rules (channel_id) VALUES ($1) RETURNING id`,
		channelID,
	).Scan(&ruleID))

	migrationSQL, err := dbmigrations.FS.ReadFile("241_channel_reasoning_effort_multipliers.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	var modelDefault string
	require.NoError(t, tx.QueryRowContext(ctx,
		`SELECT reasoning_effort_multipliers::text FROM channel_model_pricing WHERE id = $1`,
		pricingID,
	).Scan(&modelDefault))
	require.JSONEq(t, `{}`, modelDefault)

	var statsDefault string
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO channel_account_stats_model_pricing (rule_id)
VALUES ($1) RETURNING reasoning_effort_multipliers::text`, ruleID).Scan(&statsDefault))
	require.JSONEq(t, `{}`, statsDefault)

	// Persisted generic maps must survive replay unchanged.
	current := `{"low":0.75,"high":1.5,"max":3}`
	_, err = tx.ExecContext(ctx,
		`UPDATE channel_model_pricing SET reasoning_effort_multipliers = $1 WHERE id = $2`,
		current, pricingID,
	)
	require.NoError(t, err)

	for range 2 {
		_, err = tx.ExecContext(ctx, string(migrationSQL))
		require.NoError(t, err)

		var actual string
		require.NoError(t, tx.QueryRowContext(ctx,
			`SELECT reasoning_effort_multipliers::text FROM channel_model_pricing WHERE id = $1`,
			pricingID,
		).Scan(&actual))
		require.JSONEq(t, current, actual)
	}

	// Group pricing is JSON stored by the group row itself. Since slim never had
	// the upstream legacy max key, migration 241 must leave it untouched.
	groupPricing := `[{"models":["claude-opus-5-5"],"reasoning_effort_multipliers":{"high":1.5,"max":3}}]`
	var groupID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name, platform, model_pricing)
VALUES ('migration-group-reasoning', 'anthropic', $1::jsonb) RETURNING id`, groupPricing).Scan(&groupID))

	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	var actualGroupPricing string
	require.NoError(t, tx.QueryRowContext(ctx,
		`SELECT model_pricing::text FROM groups WHERE id = $1`, groupID,
	).Scan(&actualGroupPricing))
	require.JSONEq(t, groupPricing, actualGroupPricing)
}
