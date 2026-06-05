//go:build tools

// Package tools pins build-time and future-phase dependencies so that
// go mod tidy does not remove them before they are imported by production code.
package tools

import (
	_ "github.com/fsnotify/fsnotify"
	_ "github.com/go-chi/chi/v5"
	_ "github.com/golang-jwt/jwt/v5"
	_ "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5"
	_ "github.com/shirou/gopsutil/v4/cpu"
)
