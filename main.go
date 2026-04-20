package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"

	"github.com/opentalon/opentalon/pkg/plugin"
)

const defaultModel = "whisper-1"
const defaultBaseURL = "https://api.openai.com/v1"

type config struct {
	Provider string `json:"provider"`           // only "whisper" supported
	APIKey   string `json:"api_key"`            // OpenAI API key; falls back to OPENAI_API_KEY env var
	Model    string `json:"model,omitempty"`    // default "whisper-1"
	Language string `json:"language,omitempty"` // optional BCP-47 tag; omit for auto-detect
	BaseURL  string `json:"base_url,omitempty"` // override for testing; default "https://api.openai.com/v1"
}

type sttHandler struct {
	cfg config
}

func (h *sttHandler) Configure(configJSON string) error {
	if configJSON == "" {
		return nil
	}
	return json.Unmarshal([]byte(configJSON), &h.cfg)
}

func (h *sttHandler) Capabilities() plugin.CapabilitiesMsg {
	return plugin.CapabilitiesMsg{
		Name:        "stt",
		Description: "Speech-to-text transcription. Converts audio files to text using the configured provider (default: OpenAI Whisper).",
		Actions: []plugin.ActionMsg{
			{
				Name:        "transcribe",
				Description: "Transcribe a base64-encoded audio file to text.",
				Parameters: []plugin.ParameterMsg{
					{Name: "file_data", Description: "Base64-encoded audio file content", Type: "string", Required: true},
					{Name: "file_mime", Description: "MIME type of the audio file (e.g. audio/webm, audio/mp4, audio/wav)", Type: "string", Required: true},
				},
			},
		},
	}
}

func (h *sttHandler) Execute(req plugin.Request) plugin.Response {
	if req.Action != "transcribe" {
		return plugin.Response{CallID: req.ID, Error: "unknown action: " + req.Action}
	}

	fileData := req.Args["file_data"]
	fileMime := req.Args["file_mime"]
	if fileData == "" {
		return plugin.Response{CallID: req.ID, Error: "missing required arg: file_data"}
	}
	if fileMime == "" {
		return plugin.Response{CallID: req.ID, Error: "missing required arg: file_mime"}
	}

	audio, err := base64.StdEncoding.DecodeString(fileData)
	if err != nil {
		return plugin.Response{CallID: req.ID, Error: fmt.Sprintf("base64 decode: %v", err)}
	}

	transcript, err := h.transcribe(audio, fileMime)
	if err != nil {
		return plugin.Response{CallID: req.ID, Error: err.Error()}
	}
	return plugin.Response{CallID: req.ID, Content: transcript}
}

func (h *sttHandler) apiKey() string {
	if h.cfg.APIKey != "" {
		return h.cfg.APIKey
	}
	return os.Getenv("OPENAI_API_KEY")
}

func (h *sttHandler) model() string {
	if h.cfg.Model != "" {
		return h.cfg.Model
	}
	return defaultModel
}

func (h *sttHandler) baseURL() string {
	if h.cfg.BaseURL != "" {
		return h.cfg.BaseURL
	}
	return defaultBaseURL
}

// transcribe sends audio to the Whisper API and returns the transcript.
func (h *sttHandler) transcribe(audio []byte, mimeType string) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	// Part: file — use the MIME type as the Content-Type for the file part.
	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Disposition", `form-data; name="file"; filename="audio"`)
	fileHeader.Set("Content-Type", mimeType)
	fw, err := mw.CreatePart(fileHeader)
	if err != nil {
		return "", fmt.Errorf("create file part: %w", err)
	}
	if _, err := fw.Write(audio); err != nil {
		return "", fmt.Errorf("write audio: %w", err)
	}

	// Part: model
	if err := mw.WriteField("model", h.model()); err != nil {
		return "", fmt.Errorf("write model field: %w", err)
	}

	// Part: language (optional — omit for auto-detect)
	if h.cfg.Language != "" {
		if err := mw.WriteField("language", h.cfg.Language); err != nil {
			return "", fmt.Errorf("write language field: %w", err)
		}
	}

	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("close multipart: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, h.baseURL()+"/audio/transcriptions", &body)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+h.apiKey())
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("whisper request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("whisper API returned %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return result.Text, nil
}

func main() {
	h := &sttHandler{}
	if err := plugin.Serve(h); err != nil {
		os.Exit(1)
	}
}
