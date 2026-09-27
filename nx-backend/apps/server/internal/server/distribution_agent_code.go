package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"regexp"
)

const distributionAgentCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

var distributionAgentCodePattern = regexp.MustCompile(`^[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{6}$`)

func generateDistributionAgentCode() (string, error) {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	for i, value := range bytes {
		bytes[i] = distributionAgentCodeAlphabet[int(value)%len(distributionAgentCodeAlphabet)]
	}
	return string(bytes), nil
}

func validateDistributionAgentCodeFormat(code string) bool {
	return distributionAgentCodePattern.MatchString(code)
}

func generateAvailableDistributionAgentCode(ctx context.Context, db *sql.DB) (string, error) {
	for attempt := 0; attempt < 12; attempt++ {
		code, err := generateDistributionAgentCode()
		if err != nil {
			return "", err
		}
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM distribution_agents WHERE lower(agent_code)=lower($1))`, code).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
	return "", errors.New("could not allocate a unique distribution agent code")
}
