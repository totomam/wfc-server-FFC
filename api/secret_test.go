package api

import "testing"

// NET-33: an empty apiSecret disables the admin API instead of accepting an empty secret.
func TestEmptyAPISecretRefusesAll(t *testing.T) {
	old := apiSecret
	defer func() { apiSecret = old }()
	apiSecret = ""
	if authenticate(AuthInfo{Secret: ""}, RoleModerator) {
		t.Error("empty secret accepted while apiSecret is empty")
	}
	apiSecret = "real"
	if !authenticate(AuthInfo{Secret: "real"}, RoleModerator) || authenticate(AuthInfo{Secret: ""}, RoleModerator) {
		t.Error("secret check wrong")
	}
}
