package api

// NET-15: the FFC admin POST handlers share parsePost, so TestPostBodyCap covers them too.
func init() {
	postCapHandlers["/api/ffc/admin/bonus"] = HandleFFCBonus
	postCapHandlers["/api/ffc/admin/events"] = HandleFFCEvents
	postCapHandlers["/api/ffc/admin/names"] = HandleFFCNames
	postCapHandlers["/api/ffc/admin/links"] = HandleFFCLinks
	postCapHandlers["/api/ffc/admin/mail"] = HandleFFCMail
	postCapHandlers["/api/ffc/admin/motd"] = HandleFFCMotd
}
