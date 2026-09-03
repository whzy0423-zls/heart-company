package server

import (
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
)

func TestNewServerInitializesSingleAppKnowledgeCoordinator(t *testing.T) {
	server := newServer(config.Env{}, nil)
	if server.appKnowledge == nil {
		t.Fatal("app knowledge coordinator was not initialized")
	}
}
