package dto

// WalletResponse is a guild's wallet balance. Available = Total - Reserved.
type WalletResponse struct {
	GuildID   uint64 `json:"guild_id" example:"1"`
	Total     int64  `json:"total" example:"100000"`
	Reserved  int64  `json:"reserved" example:"1200"`
	Available int64  `json:"available" example:"98800"`
}
