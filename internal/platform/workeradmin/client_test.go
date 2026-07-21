package workeradmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/SamuelFan1/Axis/internal/config"
)

func TestReplaceHTTPSProbeBlacklistUsesRegionalEndpoint(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody httpsProbeRegionStatusRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewClient(config.WorkerAdminConfig{WorkerURL: server.URL, WorkerAdminSecret: "secret"})
	err := client.ReplaceHTTPSProbeBlacklist(context.Background(), "asia", []string{"NODE-B", "node-a", "node-a"})
	if err != nil {
		t.Fatalf("ReplaceHTTPSProbeBlacklist returned error: %v", err)
	}
	if gotPath != httpsProbeRegionStatusPath || gotAuth != "Bearer secret" {
		t.Fatalf("unexpected request path/auth: %s %s", gotPath, gotAuth)
	}
	if gotBody.Region != "asia" || !reflect.DeepEqual(gotBody.Nodes, []string{"node-a", "node-b"}) {
		t.Fatalf("unexpected request body: %+v", gotBody)
	}
}
