// Package model is the persistence layer. It owns the database models
// (models.go), the enum-like status constants, and the seed data (seed.go).
//
// It is deliberately behaviour-free: there is no repository abstraction over
// GORM. The service layer owns all queries, writes, and transaction boundaries,
// talking to the database (*gorm.DB) directly. Keeping the SQL next to the
// business logic keeps multi-table, row-locking transactions
// (SELECT ... FOR UPDATE) readable in one place, and avoids an indirection that
// GORM's transaction handle would leak through anyway. See docs/ADR.md
// (ADR-001) for the trade-offs.
package model
