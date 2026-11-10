package logging

import (
	"fmt"
	"strings"
	"testing"
)

// NET-22: IPs, device IDs and friend codes are dropped at any depth before an event becomes a webhook.
func TestWebhookSafe(t *testing.T) {
	in := map[string]any{
		"profile_id": 7, "ip_address": "203.0.113.9:1", "ng_device_id": 12345, "wii_number": 999, "Friend code": "1234-5678-9012",
		"peers": []map[string]string{{"profile_id": "1", "ip_address": "198.51.100.1"}, {"profile_id": "2", "ip_address": "198.51.100.2"}},
	}
	out := fmt.Sprint(webhookSafe(in))
	for _, bad := range []string{"203.0.113.9", "12345", "999", "1234-5678-9012", "198.51.100"} {
		if strings.Contains(out, bad) {
			t.Fatalf("%q survived: %s", bad, out)
		}
	}
	if !strings.Contains(out, "profile_id:7") || !strings.Contains(out, "profile_id:2") {
		t.Fatalf("too much removed: %s", out)
	}
	if _, ok := in["ip_address"]; !ok {
		t.Fatal("input map modified (the database callback gets the same map)")
	}
}
