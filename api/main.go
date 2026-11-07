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
	mux.HandleFunc("/api/ffc/leaderboard", ffcLimited(HandleFFCLeaderboard))
	mux.HandleFunc("/api/ffc/vivosaurs", ffcLimited(HandleFFCVivosaurs))
	mux.HandleFunc("/api/ffc/h2h", ffcLimited(HandleFFCH2H))
	mux.HandleFunc("/api/ffc/matches", ffcLimited(HandleFFCMatches))
	mux.HandleFunc("/api/ffc/seasons", ffcLimited(HandleFFCSeasons))
	mux.HandleFunc("/api/ffc/teams", ffcLimited(HandleFFCTeams))
	mux.HandleFunc("/api/ffc/balance", ffcLimited(HandleFFCBalance))
	mux.HandleFunc("/api/ffc/admin/flagged", HandleFFCFlagged)
	mux.HandleFunc("/api/ffc/online", ffcLimited(HandleFFCOnline))
	mux.HandleFunc("/api/ffc/admin/dashboard", HandleFFCDashboard)
	mux.HandleFunc("/api/ffc/admin/bonus", HandleFFCBonus)
	mux.HandleFunc("/api/ffc/admin/events", HandleFFCEvents)
	mux.HandleFunc("/api/ffc/plaza", ffcLimited(HandleFFCPlaza))
	mux.HandleFunc("/api/ffc/admin/ratings", HandleFFCRatings)
	mux.HandleFunc("/api/ffc/admin/links", HandleFFCLinks)
	mux.HandleFunc("/ffc/widget", HandleFFCWidget)
}
