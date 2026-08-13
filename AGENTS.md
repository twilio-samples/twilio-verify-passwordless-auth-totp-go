# Passwordless Authentication With Twilio Verify in Go

This sample demonstrates how to implement passwordless TOTP-based authentication using Twilio Verify in Go.

## Environment Variables

Copy `.env.example` to `.env`. Never commit `.env`.

```bash
cp .env.example .env
```

| Variable | Where to find | Format |
| -------- | ------------- | ------ |
| `TWILIO_ACCOUNT_SID` | Console homepage or Admin dropdown (top right) → Account Management → Keys & Credentials → API Keys & Tokens | Starts with `AC` |
| `TWILIO_AUTH_TOKEN` | Console homepage or Admin dropdown (top right) → Account Management → Keys & Credentials → API Keys & Tokens → click to reveal | 32-char string. Treat as a password. |
| `TWILIO_VERIFY_SERVICE_SID` | Console → Verify → Services | Starts with `VA` |

## Commands

```bash
# Install
go mod download

# Run
go run main.go
```

## Project Structure

- `main.go` — application entry point, HTTP handlers, Twilio Verify API calls
- `templates/` — HTML templates for the TOTP registration and verification flows
- `.env.example` — environment variable template
- `go.mod` — Go module definition and dependencies

## Agent Boundaries

**Always:**
- Confirm `.env` is configured before running any command
- Use the Environment Variables section to guide the user to each credential — don't ask them to find values without direction
- Confirm the app is running before asking the user to test it

**Never:**
- Run the app with missing or placeholder credentials
- Hardcode credentials or phone numbers in source files
- Skip the `cp .env.example .env` step

## Verify It's Working

1. Run `go run main.go` and navigate to `http://localhost:8080`.
2. Enter a username to register a TOTP factor — you'll be shown a QR code to scan with an authenticator app (e.g. Google Authenticator). After scanning, enter the generated code to verify enrollment, then use the code at login to confirm the flow is working end-to-end.

## Twilio Resources

- [Twilio Console](https://console.twilio.com) — credentials, phone numbers, webhook configuration
- [Twilio Verify Docs](https://www.twilio.com/docs/verify)
- [twilio-go SDK](https://www.twilio.com/docs/libraries/go)
