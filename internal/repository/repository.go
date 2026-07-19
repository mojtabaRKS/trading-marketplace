// Package repository holds the persistence models and data-access types.
// The service layer depends on these repositories; it never touches *gorm.DB
// or SQL directly.
package repository

import "gorm.io/gorm"

// Repositories is the aggregate of all repositories, sharing one DB handle.
// Concrete repositories (Guild, Wallet, Item, Auction, ...) are added in the
// steps that need them.
type Repositories struct {
	db *gorm.DB
}

// New builds the repository aggregate around a GORM handle.
func New(db *gorm.DB) *Repositories {
	return &Repositories{db: db}
}

// DB exposes the underlying handle for transaction orchestration in the
// service layer.
func (r *Repositories) DB() *gorm.DB {
	return r.db
}
