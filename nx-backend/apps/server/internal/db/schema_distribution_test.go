package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaContainsDistributionModelAndIdempotencyGuards(t *testing.T) {
	b, err := os.ReadFile("schema.sql")
	if err != nil { t.Fatal(err) }
	s := string(b)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS distribution_agents",
		"CREATE TABLE IF NOT EXISTS distribution_user_relations",
		"CREATE TABLE IF NOT EXISTS distribution_commission_records",
		"CREATE TABLE IF NOT EXISTS distribution_settlements",
		"UNIQUE (order_id, agent_id)",
		"idx_distribution_user_relations_direct_agent",
	} {
		if !strings.Contains(s, fragment) { t.Errorf("schema missing %q", fragment) }
	}
}
