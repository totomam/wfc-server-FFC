package api

import (
	"net/http"
	"wwfc/common"
	"wwfc/database"
)

var (
	db database.Connection

	apiSecret string
)

func StartServer(reload bool) {
	// Get config
	config := common.GetConfig()

	apiSecret = config.APISecret

	// Start SQL
	db = database.Start(config)

	db.RegisterEvents(config, []string{
		"profile_kicked",
		"profile_banned",
		"profile_unbanned",
	})
}

func Shutdown() {
	db.Close()
}

func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/groups", HandleGroups)
	mux.HandleFunc("/api/stats", HandleStats)
	mux.HandleFunc("/api/ban", HandleBan)
	mux.HandleFunc("/api/unban", HandleUnban)
	mux.HandleFunc("/api/kick", HandleKick)
	mux.HandleFunc("/api/baninfo", HandleBanInfo)
	mux.HandleFunc("/api/ffc/leaderboard", HandleFFCLeaderboard)
	mux.HandleFunc("/api/ffc/vivosaurs", HandleFFCVivosaurs)
	mux.HandleFunc("/api/ffc/h2h", HandleFFCH2H)
	mux.HandleFunc("/api/ffc/matches", HandleFFCMatches)
	mux.HandleFunc("/api/ffc/seasons", HandleFFCSeasons)
	mux.HandleFunc("/api/ffc/admin/flagged", HandleFFCFlagged)
}
