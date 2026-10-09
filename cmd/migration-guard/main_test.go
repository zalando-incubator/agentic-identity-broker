package main

import (
	"strings"
	"testing"
)

func TestMigrationArgs(t *testing.T) {
	const (
		path = "/app/migrations"
		url  = "postgres://migration_user:private-password@postgres:5432/broker?sslmode=require"
	)

	tests := []struct {
		name    string
		args    []string
		wantDir string
		wantURL string
		wantErr bool
	}{
		{
			name:    "Helm job arguments",
			args:    []string{"-path", path, "-database", url, "up"},
			wantDir: path,
			wantURL: url,
		},
		{
			name:    "reversed flag order",
			args:    []string{"-database", url, "-path", path, "up"},
			wantDir: path,
			wantURL: url,
		},
		{
			name:    "equals syntax",
			args:    []string{"-database=" + url, "-path=" + path, "up"},
			wantDir: path,
			wantURL: url,
		},
		{
			name:    "mixed syntax",
			args:    []string{"-path=" + path, "-database", url, "down", "1"},
			wantDir: path,
			wantURL: url,
		},
		{
			name:    "missing path flag",
			args:    []string{"-database", url, "up"},
			wantErr: true,
		},
		{
			name:    "missing database flag",
			args:    []string{"-path", path, "up"},
			wantErr: true,
		},
		{
			name:    "path value absent at end",
			args:    []string{"-database", url, "-path"},
			wantErr: true,
		},
		{
			name:    "database value absent at end",
			args:    []string{"-path", path, "-database"},
			wantErr: true,
		},
		{
			name:    "path value is another flag",
			args:    []string{"-path", "-database", url, "up"},
			wantErr: true,
		},
		{
			name:    "database value is another flag",
			args:    []string{"-database", "-path", path, "up"},
			wantErr: true,
		},
		{
			name:    "empty path equals value",
			args:    []string{"-path=", "-database", url, "up"},
			wantErr: true,
		},
		{
			name:    "empty database equals value",
			args:    []string{"-path", path, "-database=", "up"},
			wantErr: true,
		},
		{
			name:    "duplicate path forms",
			args:    []string{"-path", path, "-database", url, "-path=/another/directory", "up"},
			wantErr: true,
		},
		{
			name:    "duplicate database forms",
			args:    []string{"-path", path, "-database=" + url, "-database", "postgres://other:second-secret@postgres/broker", "up"},
			wantErr: true,
		},
		{
			name:    "alternate source flag is rejected",
			args:    []string{"-path", path, "-database", url, "-source", "file:///tmp/alternate", "up"},
			wantErr: true,
		},
		{
			name:    "alternate source equals flag is rejected",
			args:    []string{"-path", path, "-database", url, "--source=file:///tmp/alternate", "up"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, databaseURL, err := migrationArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("migrationArgs(%q) returned no error", tt.args)
				}
				for _, secret := range []string{"private-password", "second-secret", url} {
					if strings.Contains(err.Error(), secret) {
						t.Errorf("migrationArgs error exposed database credentials: %q", err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("migrationArgs(%q): %v", tt.args, err)
			}
			if dir != tt.wantDir || databaseURL != tt.wantURL {
				t.Errorf("migrationArgs() = (%q, %q), want (%q, %q)", dir, databaseURL, tt.wantDir, tt.wantURL)
			}
		})
	}
}
