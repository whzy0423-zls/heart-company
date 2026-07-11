package testutil

import "testing"

func TestValidateIsolatedPostgresDSN(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		wantErr bool
	}{
		{name: "dedicated test database", dsn: "postgres://nx:nx@localhost:5432/nx_admin_test?sslmode=disable"},
		{name: "postgresql scheme", dsn: "postgresql://nx:nx@localhost:5432/workflow_test"},
		{name: "empty", dsn: "", wantErr: true},
		{name: "production database", dsn: "postgres://nx:nx@localhost:5432/nx_admin", wantErr: true},
		{name: "test appears only in host", dsn: "postgres://nx:nx@test.example.com:5432/nx_admin", wantErr: true},
		{name: "dbname query overrides safe path", dsn: "postgres://nx:nx@localhost:5432/safe_test?dbname=production", wantErr: true},
		{name: "database query overrides safe path", dsn: "postgres://nx:nx@localhost:5432/safe_test?database=production", wantErr: true},
		{name: "empty dbname query overrides safe path", dsn: "postgres://nx:nx@localhost:5432/safe_test?dbname=", wantErr: true},
		{name: "empty database query overrides safe path", dsn: "postgres://nx:nx@localhost:5432/safe_test?database=", wantErr: true},
		{name: "unsupported scheme", dsn: "mysql://nx:nx@localhost:3306/nx_admin_test", wantErr: true},
		{name: "missing database", dsn: "postgres://nx:nx@localhost:5432/", wantErr: true},
		{name: "malformed", dsn: "://not-a-url", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIsolatedPostgresDSN(tt.dsn)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateIsolatedPostgresDSN(%q) error = %v, wantErr %v", tt.dsn, err, tt.wantErr)
			}
		})
	}
}
