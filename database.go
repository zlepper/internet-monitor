package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/tern/migrate"
)



func connectToDatabase(config *Config) (*pgx.Conn, error) {
	conn, err := pgx.Connect(context.Background(), config.ConnectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	err = conn.Ping(context.Background())
	if err != nil {
		return nil, fmt.Errorf("ping failed: %w", err)
	}

	err = runMigrations(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to run migrations")
	}

	return conn, nil
}

func runMigrations(conn *pgx.Conn) error  {
	m, err := migrate.NewMigrator(context.Background(), conn, "schema_version")
	if err != nil {
		return fmt.Errorf("failed to initialize migrator: %w", err)
	}
	err = m.LoadMigrations("./migrations")
	if err != nil {
		return fmt.Errorf("failed to load migrations")
	}

	err = m.Migrate(context.Background())
	if err != nil {
		return fmt.Errorf("failed to run actual migrations: %w", err)
	}

	return nil
}
