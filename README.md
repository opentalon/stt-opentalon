# stt-opentalon

[![CI](https://github.com/opentalon/stt-opentalon/actions/workflows/ci.yml/badge.svg)](https://github.com/opentalon/stt-opentalon/actions/workflows/ci.yml)

Speech-to-text plugin for [OpenTalon](https://github.com/opentalon/opentalon). Transcribes audio messages to text before they reach the LLM.

Supports **OpenAI Whisper** out of the box. Language is auto-detected when not configured.

## How it works

When a user sends a voice/audio message, OpenTalon passes the audio file to this plugin as a base64-encoded argument. The plugin calls the Whisper API and returns the transcript, which OpenTalon uses as the user message going into the LLM.

```
User sends audio → stt-opentalon → transcript → LLM
```

## Configuration

```yaml
plugins:
  stt:
    command: ./stt-opentalon
    config:
      provider: whisper
      api_key: ${OPENAI_API_KEY}   # or set OPENAI_API_KEY env var
      model: whisper-1             # optional, default: whisper-1
      language: en                 # optional, omit for auto-detect

orchestrator:
  content_preparers:
    - plugin: stt
      action: transcribe
      stt: true
      fail_open: true  # continue without transcript if STT fails
```

### Config fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `provider` | no | `whisper` | STT provider. Only `whisper` is supported. |
| `api_key` | no* | `$OPENAI_API_KEY` | OpenAI API key. Falls back to the `OPENAI_API_KEY` environment variable. |
| `model` | no | `whisper-1` | Whisper model name. |
| `language` | no | auto-detect | BCP-47 language tag (e.g. `en`, `uk`, `de`). Omit to let Whisper detect automatically. |

\* Required if `OPENAI_API_KEY` env var is not set.

## Supported audio formats

All formats accepted by the Whisper API: `audio/webm`, `audio/mp4`, `audio/wav`, `audio/mpeg`, `audio/ogg`, `audio/flac`, and others.

## Build

```sh
make build   # produces ./stt-opentalon binary
make test    # run tests
make lint    # run golangci-lint
```

## Channel integration

For YAML-based channels (Telegram, WhatsApp, etc.), add a `media` rule to fetch voice messages as binary files:

```yaml
# Telegram example
inbound:
  media:
    - when: "voice"
      resolve:
        mime_type: "audio/ogg"
        steps:
          - url: "https://api.telegram.org/bot{{env.TOKEN}}/getFile?file_id={{event.voice.file_id}}"
            store: { file_path: "result.file_path" }
          - url: "https://api.telegram.org/file/bot{{env.TOKEN}}/{{resolve.file_path}}"
```

The resolved audio binary is passed to the STT plugin automatically.
