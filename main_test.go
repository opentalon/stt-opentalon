package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opentalon/opentalon/pkg/plugin"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// --- Capabilities ---

func TestCapabilities(t *testing.T) {
	h := &sttHandler{}
	caps := h.Capabilities()
	if caps.Name != "stt" {
		t.Errorf("Name = %q, want stt", caps.Name)
	}
	if len(caps.Actions) != 1 || caps.Actions[0].Name != "transcribe" {
		t.Errorf("Actions = %v", caps.Actions)
	}
	params := caps.Actions[0].Parameters
	if len(params) != 2 {
		t.Fatalf("expected 2 parameters, got %d", len(params))
	}
	names := map[string]bool{params[0].Name: true, params[1].Name: true}
	if !names["file_data"] || !names["file_mime"] {
		t.Errorf("parameters = %v, want file_data and file_mime", params)
	}
}

// --- Configure ---

func TestConfigure_SetsFields(t *testing.T) {
	h := &sttHandler{}
	cfg := `{"provider":"whisper","api_key":"sk-test","model":"whisper-1","language":"uk"}`
	if err := h.Configure(cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if h.cfg.APIKey != "sk-test" {
		t.Errorf("APIKey = %q", h.cfg.APIKey)
	}
	if h.cfg.Language != "uk" {
		t.Errorf("Language = %q", h.cfg.Language)
	}
}

func TestConfigure_Empty_NoError(t *testing.T) {
	h := &sttHandler{}
	if err := h.Configure(""); err != nil {
		t.Fatalf("empty config should not error: %v", err)
	}
}

// --- Execute ---

func TestExecute_UnknownAction(t *testing.T) {
	h := &sttHandler{}
	resp := h.Execute(plugin.Request{ID: "r1", Action: "unknown"})
	if resp.Error == "" {
		t.Error("expected error for unknown action")
	}
}

func TestExecute_MissingFileData(t *testing.T) {
	h := &sttHandler{}
	resp := h.Execute(plugin.Request{ID: "r1", Action: "transcribe", Args: map[string]string{"file_mime": "audio/webm"}})
	if resp.Error == "" {
		t.Error("expected error for missing file_data")
	}
}

func TestExecute_MissingFileMime(t *testing.T) {
	h := &sttHandler{}
	resp := h.Execute(plugin.Request{ID: "r1", Action: "transcribe", Args: map[string]string{"file_data": b64("audio")}})
	if resp.Error == "" {
		t.Error("expected error for missing file_mime")
	}
}

func TestExecute_InvalidBase64(t *testing.T) {
	h := &sttHandler{}
	resp := h.Execute(plugin.Request{ID: "r1", Action: "transcribe", Args: map[string]string{
		"file_data": "not-valid-base64!!!",
		"file_mime": "audio/webm",
	}})
	if resp.Error == "" {
		t.Error("expected error for invalid base64")
	}
}

// --- transcribe (against mock HTTP server) ---

func mockWhisperServer(t *testing.T, wantModel, wantLanguage string, response interface{}, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		if got := r.FormValue("model"); got != wantModel {
			t.Errorf("model = %q, want %q", got, wantModel)
		}
		if wantLanguage != "" {
			if got := r.FormValue("language"); got != wantLanguage {
				t.Errorf("language = %q, want %q", got, wantLanguage)
			}
		}
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(response)
	}))
}

func TestTranscribe_Success(t *testing.T) {
	srv := mockWhisperServer(t, "whisper-1", "", map[string]string{"text": "hello world"}, http.StatusOK)
	defer srv.Close()

	h := &sttHandler{cfg: config{APIKey: "sk-test", BaseURL: srv.URL}}
	resp := h.Execute(plugin.Request{ID: "r1", Action: "transcribe", Args: map[string]string{
		"file_data": b64("audio-bytes"),
		"file_mime": "audio/webm",
	}})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Content != "hello world" {
		t.Errorf("Content = %q, want hello world", resp.Content)
	}
	if resp.CallID != "r1" {
		t.Errorf("CallID = %q, want r1", resp.CallID)
	}
}

func TestTranscribe_WithLanguage(t *testing.T) {
	srv := mockWhisperServer(t, "whisper-1", "uk", map[string]string{"text": "привіт"}, http.StatusOK)
	defer srv.Close()

	h := &sttHandler{cfg: config{APIKey: "sk-test", Language: "uk", BaseURL: srv.URL}}
	resp := h.Execute(plugin.Request{ID: "r1", Action: "transcribe", Args: map[string]string{
		"file_data": b64("audio-bytes"),
		"file_mime": "audio/mp4",
	}})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Content != "привіт" {
		t.Errorf("Content = %q, want привіт", resp.Content)
	}
}

func TestTranscribe_CustomModel(t *testing.T) {
	srv := mockWhisperServer(t, "whisper-large", "", map[string]string{"text": "ok"}, http.StatusOK)
	defer srv.Close()

	h := &sttHandler{cfg: config{APIKey: "sk-test", Model: "whisper-large", BaseURL: srv.URL}}
	resp := h.Execute(plugin.Request{ID: "r1", Action: "transcribe", Args: map[string]string{
		"file_data": b64("audio"),
		"file_mime": "audio/ogg",
	}})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
}

func TestTranscribe_APIError(t *testing.T) {
	srv := mockWhisperServer(t, "whisper-1", "", map[string]string{"error": "bad request"}, http.StatusBadRequest)
	defer srv.Close()

	h := &sttHandler{cfg: config{APIKey: "sk-test", BaseURL: srv.URL}}
	resp := h.Execute(plugin.Request{ID: "r1", Action: "transcribe", Args: map[string]string{
		"file_data": b64("audio"),
		"file_mime": "audio/webm",
	}})
	if resp.Error == "" {
		t.Error("expected error for non-200 response")
	}
}

func TestTranscribe_DefaultModel(t *testing.T) {
	h := &sttHandler{}
	if h.model() != defaultModel {
		t.Errorf("default model = %q, want %q", h.model(), defaultModel)
	}
}

func TestTranscribe_NoLanguage_OmitsField(t *testing.T) {
	// When language is empty, the language field must NOT be sent to the API.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		if lang := r.FormValue("language"); lang != "" {
			// language field should be absent when not configured
			http.Error(w, "language should not be set", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "auto detected"})
	}))
	defer srv.Close()

	h := &sttHandler{cfg: config{APIKey: "sk-test", BaseURL: srv.URL}}
	resp := h.Execute(plugin.Request{ID: "r1", Action: "transcribe", Args: map[string]string{
		"file_data": b64("audio"),
		"file_mime": "audio/webm",
	}})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Content != "auto detected" {
		t.Errorf("Content = %q, want auto detected", resp.Content)
	}
}
