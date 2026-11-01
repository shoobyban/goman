package goman

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFormatBodyBudget(t *testing.T) {
	for _, raw := range []string{
		strings.Repeat("[", 2100) + "0" + strings.Repeat("]", 2100),
		strings.Repeat("[", 9999) + "0" + strings.Repeat("]", 9999),
		`{"escaped":"\\\"[{},:]","nested":[{},[1,2]]}`,
		`{"key":"` + strings.Repeat("x", int(MaxBodyBytes)-10) + `"}`,
	} {
		formatted, valid := FormatBody([]byte(raw))
		if !valid || !json.Valid([]byte(formatted)) || int64(len(formatted)) > MaxBodyBytes {
			t.Fatalf("format budget: input=%d output=%d valid=%t", len(raw), len(formatted), valid)
		}
		if jsonIndentFits([]byte(raw)) {
			var expected bytes.Buffer
			if err := json.Indent(&expected, []byte(raw), "", "  "); err != nil || formatted != expected.String() {
				t.Fatalf("normal formatting changed: %v", err)
			}
		} else if formatted != raw {
			t.Fatal("oversized formatting did not preserve original JSON")
		}
	}
}

func TestExecuteHostOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.WriteString(writer, request.Host)
	}))
	defer server.Close()
	client := NewClient(time.Second, false)
	defer client.CloseIdleConnections()
	for _, key := range []string{"Host", "host", "HOST"} {
		result := Execute(context.Background(), client, Request{Method: "GET", URL: server.URL, Headers: map[string]string{key: "virtual.example:8080"}})
		if result.Err != nil || result.Body != "virtual.example:8080" {
			t.Fatalf("Host override %q: %+v", key, result)
		}
	}
}

func TestExecutePreservesMethod(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Method", request.Method)
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
		}
		if request.Method == "HEAD" && len(body) != 0 {
			t.Error("HEAD sent a body")
		}
		if request.Method != "HEAD" && string(body) != "payload" {
			t.Error("custom method lost body")
		}
	}))
	defer server.Close()
	client := NewClient(time.Second, false)
	defer client.CloseIdleConnections()
	for _, method := range []string{"HEAD", "OPTIONS", "PROPFIND", "custom"} {
		result := Execute(context.Background(), client, Request{Method: method, URL: server.URL, Body: "payload"})
		if result.Err != nil || result.Headers.Get("X-Method") != method {
			t.Fatalf("method %q changed: %+v", method, result)
		}
	}
}

func TestNestedJSONResponseAndWebhook(t *testing.T) {
	raw := strings.Repeat("[", 2100) + "0" + strings.Repeat("]", 2100)
	endpoint := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.WriteString(writer, raw)
	}))
	defer endpoint.Close()
	client := NewClient(time.Second, false)
	defer client.CloseIdleConnections()
	result := Execute(context.Background(), client, Request{Method: "GET", URL: endpoint.URL})
	if result.Err != nil || result.Body != raw || !result.IsJSON {
		t.Fatalf("nested response: err=%v size=%d", result.Err, len(result.Body))
	}
	server := &WebhookServer{}
	recorder := httptest.NewRecorder()
	server.receive(recorder, httptest.NewRequest("POST", "/nested", strings.NewReader(raw)))
	hooks := server.Hooks()
	if recorder.Code != http.StatusOK || len(hooks) != 1 || hooks[0].Body != raw || hooks[0].RawBody != raw || server.historyBytes > MaxHistoryBytes {
		t.Fatalf("nested webhook: status=%d hooks=%d bytes=%d", recorder.Code, len(hooks), server.historyBytes)
	}
}

func TestCollectionsCompatibility(t *testing.T) {
	store := Storage{Dir: filepath.Join(t.TempDir(), ".ghostman")}
	collections, err := store.Load()
	if err != nil || len(collections) != 1 || len(collections[0].Requests) != 3 {
		t.Fatalf("defaults: %v %v", collections, err)
	}
	collections[0].Requests = append(collections[0].Requests, Request{Name: "Unicode \u2603", Method: "PATCH", URL: "http://localhost/test", Body: "\n{\"value\":42}\n", Headers: map[string]string{"Authorization": "Bearer local"}})
	if err := store.Save(collections); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil || loaded[0].Requests[3].Body != collections[0].Requests[3].Body {
		t.Fatalf("YAML roundtrip: %v %v", loaded, err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "collections.yaml"), []byte("not: [valid"), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.Load()
	if err != nil || loaded[0].Requests[3].Headers["Authorization"] != "Bearer local" {
		t.Fatalf("JSON fallback: %v %v", loaded, err)
	}
	if err := store.Save([]Collection{}); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.Load()
	if err != nil || len(loaded) != 0 {
		t.Fatalf("empty collections: %v %v", loaded, err)
	}
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := (Storage{Dir: blocked}).Save(collections); err == nil {
		t.Fatal("storage failure hidden")
	}
}

func TestWorkspaceStorageMigration(t *testing.T) {
	legacyCollections := []Collection{{Name: "Original", Requests: []Request{{Name: "Saved request", Method: "PATCH", URL: "https://example.test", Body: "\nkeep payload\n", Headers: map[string]string{"User-Agent": "my-client", "Authorization": "opaque-value"}}}}}
	for _, fixture := range []struct {
		name                  string
		legacy, current       bool
		expected              []Collection
		jsonOnly, emptyLegacy bool
	}{
		{"fresh", false, false, DefaultCollections(), false, false},
		{"legacy", true, false, legacyCollections, false, false},
		{"JSON-only legacy", true, false, legacyCollections, true, false},
		{"empty legacy", true, false, []Collection{}, false, true},
		{"current wins", true, true, []Collection{}, false, false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			workspace := t.TempDir()
			legacy := Storage{Dir: filepath.Join(workspace, ".ghostman")}
			original := map[string]string{}
			if fixture.legacy {
				data := legacyCollections
				if fixture.emptyLegacy {
					data = []Collection{}
				}
				if err := legacy.Save(data); err != nil {
					t.Fatal(err)
				}
				if fixture.jsonOnly {
					if err := os.Remove(filepath.Join(legacy.Dir, "collections.yaml")); err != nil {
						t.Fatal(err)
					}
				}
				for _, format := range []string{"yaml", "json"} {
					if fixture.jsonOnly && format == "yaml" {
						continue
					}
					data, err := os.ReadFile(filepath.Join(legacy.Dir, "collections."+format))
					if err != nil {
						t.Fatal(err)
					}
					original[format] = string(data)
				}
			}
			if fixture.current {
				if err := (Storage{Dir: filepath.Join(workspace, ".goman")}).Save(fixture.expected); err != nil {
					t.Fatal(err)
				}
			}
			store, collections, err := LoadWorkspace(workspace)
			if err != nil || store.Dir != filepath.Join(workspace, ".goman") || !reflect.DeepEqual(collections, fixture.expected) {
				t.Fatalf("load workspace: %+v %v", collections, err)
			}
			stored, err := store.Load()
			if err != nil || !reflect.DeepEqual(stored, fixture.expected) {
				t.Fatalf("saved collections: %+v %v", stored, err)
			}
			for format, expected := range original {
				data, err := os.ReadFile(filepath.Join(legacy.Dir, "collections."+format))
				if err != nil || string(data) != expected {
					t.Fatalf("legacy %s changed: %v", format, err)
				}
			}
		})
	}
	for _, directory := range []string{".goman", ".ghostman"} {
		t.Run("corrupt "+directory, func(t *testing.T) {
			workspace := t.TempDir()
			if directory == ".goman" {
				if err := (Storage{Dir: filepath.Join(workspace, ".ghostman")}).Save(legacyCollections); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(filepath.Join(workspace, directory), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workspace, directory, "collections.yaml"), []byte("not: [valid"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := LoadWorkspace(workspace); err == nil {
				t.Fatal("corrupt workspace was silently replaced")
			}
			if _, err := os.Stat(filepath.Join(workspace, ".goman", "collections.json")); !os.IsNotExist(err) {
				t.Fatalf("unexpected default collections: %v", err)
			}
		})
	}
}

func TestExecuteRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/redirect" {
			http.Redirect(writer, request, "/ok", http.StatusFound)
			return
		}
		raw, _ := io.ReadAll(request.Body)
		if request.Method == "GET" && len(raw) != 0 {
			t.Error("GET sent a payload")
		}
		if request.Method != "GET" && (string(raw) != `{"hello":"world"}` || request.Header.Get("Content-Type") != "application/json") {
			t.Error("payload or JSON header missing")
		}
		if request.Header.Get("X-Test") != "kept" || request.Header.Get("User-Agent") != "custom" {
			t.Error("stored headers lost")
		}
		writer.WriteHeader(http.StatusTeapot)
		io.WriteString(writer, `{"ok":true}`)
	}))
	defer server.Close()
	client := NewClient(time.Second, false)
	defer client.CloseIdleConnections()
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			result := Execute(context.Background(), client, Request{Method: method, URL: server.URL + "/ok", Body: `{"hello":"world"}`, Headers: map[string]string{"X-Test": "kept", "user-agent": "custom"}})
			if result.Err != nil || result.StatusCode != 418 || !result.IsJSON || result.SizeBytes != 11 || result.Elapsed <= 0 {
				t.Fatalf("response: %+v", result)
			}
		})
	}
	if result := Execute(context.Background(), client, Request{Method: "GET", URL: server.URL + "/redirect", Headers: map[string]string{"X-Test": "kept", "User-Agent": "custom"}}); result.Err != nil || result.StatusCode != 418 {
		t.Fatalf("redirect: %+v", result)
	}
	if result := Execute(context.Background(), client, Request{URL: "ftp://localhost"}); result.Err == nil {
		t.Fatal("invalid URL accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := Execute(ctx, client, Request{URL: server.URL}); result.Err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestDefaultClientIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("User-Agent") != "goman/0.1.0" {
			t.Errorf("unexpected default User-Agent: %q", request.Header.Get("User-Agent"))
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := NewClient(time.Second, false)
	defer client.CloseIdleConnections()
	if result := Execute(context.Background(), client, Request{Method: "GET", URL: server.URL}); result.Err != nil || result.StatusCode != http.StatusNoContent {
		t.Fatalf("request: %+v", result)
	}
}

func TestWebhookLifecycle(t *testing.T) {
	received := make(chan Hook, 1)
	server := &WebhookServer{OnHook: func(hook Hook) { received <- hook }}
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())
	response, err := http.Post("http://"+server.Address()+"/hook?event=test", "application/json", strings.NewReader(`{"ping":"pong"}`))
	if err != nil {
		t.Fatal(err)
	}
	var acknowledgement map[string]string
	err = json.NewDecoder(response.Body).Decode(&acknowledgement)
	response.Body.Close()
	if err != nil || acknowledgement["source"] != "goman" || acknowledgement["status"] != "ok" {
		t.Fatalf("webhook acknowledgement: %v %v", acknowledgement, err)
	}
	select {
	case hook := <-received:
		if hook.Path != "/hook?event=test" || hook.RawBody != `{"ping":"pong"}` || hook.ID != 1 || hook.Headers.Get("Content-Type") != "application/json" {
			t.Fatalf("capture: %+v", hook)
		}
	case <-time.After(time.Second):
		t.Fatal("hook not delivered")
	}
	if err := server.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if server.Running() || len(server.Hooks()) != 1 {
		t.Fatal("stop lost history or running state")
	}
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
}

func TestTLSAndResponseLimit(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/large" {
			io.WriteString(writer, strings.Repeat("x", int(MaxBodyBytes+1)))
			return
		}
		io.WriteString(writer, "plain text")
	}))
	defer server.Close()
	secure := NewClient(time.Second, false)
	defer secure.CloseIdleConnections()
	if result := Execute(context.Background(), secure, Request{Method: "GET", URL: server.URL}); result.Err == nil {
		t.Fatal("untrusted TLS certificate accepted by default")
	}
	insecure := NewClient(time.Second, true)
	defer insecure.CloseIdleConnections()
	if result := Execute(context.Background(), insecure, Request{Method: "GET", URL: server.URL}); result.Err != nil || result.Body != "plain text" || result.IsJSON {
		t.Fatalf("explicit insecure: %+v", result)
	}
	if result := Execute(context.Background(), insecure, Request{Method: "GET", URL: server.URL + "/large"}); result.Err == nil || !strings.Contains(result.Err.Error(), "exceeds") {
		t.Fatal("large response accepted")
	}
}

func TestWebhookLimitsAndShutdown(t *testing.T) {
	server := &WebhookServer{}
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())
	other := &WebhookServer{}
	if err := other.Start(server.Address()); err == nil {
		other.Stop(context.Background())
		t.Fatal("bind conflict hidden")
	}
	for index := range MaxHooks + 1 {
		request := httptest.NewRequest("POST", "/history", strings.NewReader("body"))
		server.receive(httptest.NewRecorder(), request)
		if index == 0 {
			snapshot := server.Hooks()
			snapshot[0].Headers.Set("X-Changed", "true")
			if server.Hooks()[0].Headers.Get("X-Changed") != "" {
				t.Fatal("history snapshot aliases stored headers")
			}
		}
	}
	if hooks := server.Hooks(); len(hooks) != MaxHooks || hooks[0].ID != 2 {
		t.Fatal("history count limit failed")
	}
	for range 6 {
		server.receive(httptest.NewRecorder(), httptest.NewRequest("POST", "/budget", strings.NewReader(strings.Repeat("x", 1<<20))))
	}
	if hooks := server.Hooks(); len(hooks) != 4 || server.historyBytes > MaxHistoryBytes {
		t.Fatal("history byte budget failed")
	}
	large := httptest.NewRecorder()
	server.receive(large, httptest.NewRequest("POST", "/large", strings.NewReader(strings.Repeat("x", int(MaxBodyBytes+1)))))
	if large.Code != http.StatusRequestEntityTooLarge {
		t.Fatal("oversized webhook accepted")
	}
	reader, writer := io.Pipe()
	go func() { io.WriteString(writer, `{"chunked":true}`); writer.Close() }()
	request, err := http.NewRequest("POST", "http://"+server.Address()+"/chunked", reader)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if hooks := server.Hooks(); hooks[len(hooks)-1].RawBody != `{"chunked":true}` {
		t.Fatal("chunked webhook not captured")
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	server.OnHook = func(Hook) { close(entered); <-release; server.Hooks() }
	go func() {
		response, err := http.Post("http://"+server.Address()+"/shutdown", "text/plain", strings.NewReader("x"))
		if err == nil {
			response.Body.Close()
		}
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() { done <- server.Stop(ctx) }()
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("shutdown waited on its own state lock: %v", err)
	}
}
