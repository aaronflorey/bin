package providers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
)

func TestGitHubFetchAppliesAndReturnsSelectionIntent(t *testing.T) {
	resetGitHubReleaseCache(t)
	firstAssetName := platformFixtureName("tool-v2.0.0-linux-amd64-musl", "tool-v2.0.0-windows-amd64-gnu"+genericScriptExtension())
	secondAssetName := platformFixtureName("tool-v2.0.0-linux-amd64-glibc", "tool-v2.0.0-windows-amd64-msvc"+genericScriptExtension())
	payload := genericRunnablePayload(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/tool/releases/latest":
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v2.0.0", "assets": []map[string]string{
				{"name": firstAssetName, "url": server.URL + "/musl"},
				{"name": secondAssetName, "url": server.URL + "/glibc"},
			}})
		case "/musl":
			_, _ = w.Write(payload)
		case "/glibc":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	intent := &config.SelectionDescriptor{LogicalProduct: "tool"}
	file, err := newTestGitHubProvider(t, server.URL, "acme", "tool", "").Fetch(&FetchOpts{SelectionIntent: intent, NonInteractive: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := closeFileData(file.Data); err != nil {
			t.Errorf("close fetched file: %v", err)
		}
	})
	if file.SelectionIntent == nil || file.SelectionIntent.LogicalProduct != "tool" || file.SelectionIntent.Target == nil || file.SelectionIntent.Target.ABI == "" {
		t.Fatalf("fetched file provenance/intent = %#v", file)
	}
}
