package main

import (
	"encoding/hex"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/joho/godotenv"
	"github.com/skip2/go-qrcode"
	"github.com/twilio/twilio-go"
	verify "github.com/twilio/twilio-go/rest/verify/v2"
)

var sessionManager *scs.SessionManager

// This function generates the Factor seed
// It was borrowed from https://stackoverflow.com/a/46909816
func generateSeed(n int) string {
	b := make([]byte, (n+1)/2)

	src := rand.New(rand.NewSource(time.Now().UnixNano()))
	if _, err := src.Read(b); err != nil {
		panic(err)
	}

	return hex.EncodeToString(b)[:n]
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	sessionManager = scs.New()
	sessionManager.Lifetime = 24 * time.Hour

	app := Application{
		twilioRestClient: twilio.NewRestClientWithParams(twilio.ClientParams{
			Username: os.Getenv("TWILIO_ACCOUNT_SID"),
			Password: os.Getenv("TWILIO_AUTH_TOKEN"),
		}),
		verifyServiceSid: os.Getenv("TWILIO_VERIFY_SERVICE_SID"),
	}

	fileServer := http.FileServer(http.Dir("./assets/"))

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static", fileServer))

	mux.HandleFunc("GET /", app.displayCreateTotpFactorForm)
	mux.HandleFunc("POST /", app.processCreateTotpFactorForm)

	mux.HandleFunc("GET /challenge", app.displayVerifyUserForm)
	mux.HandleFunc("POST /challenge", app.processVerifyUserForm)

	mux.HandleFunc("GET /token", app.showQRCodeForm)
	mux.HandleFunc("POST /token", app.processQRCodeForm)

	log.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", sessionManager.LoadAndSave(mux)); err != nil {
		log.Printf("Failed to start server: %v\n", err)
	}
}

type Application struct {
	twilioRestClient *twilio.RestClient
	verifyServiceSid string
}

// This renders the form where the user can enter their username to set up
// TOTP-based 2FA.
func (app *Application) displayCreateTotpFactorForm(w http.ResponseWriter, r *http.Request) {
	signInTmpl, err := template.ParseFiles("./templates/enter-username.tmpl")
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err = signInTmpl.Execute(w, nil)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// This processes the handleShowCreateNewFactorForm and creates a new TOTP
// factor with Twilio. On success, it redirects the user to handleCreateQRCode.
func (app *Application) processCreateTotpFactorForm(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Fatal(err)
	}
	username := r.Form.Get("username")

	seed := generateSeed(55)
	sessionManager.Put(r.Context(), "seed", seed)

	params := &verify.CreateNewFactorParams{}
	params.SetFriendlyName(username)
	params.SetFactorType("totp")

	resp, err := app.twilioRestClient.VerifyV2.CreateNewFactor(
		app.verifyServiceSid,
		seed,
		params,
	)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if resp.Binding == nil {
		log.Printf("Binding not available: %v\n", resp.Binding)
	}

	if binding, ok := (*resp.Binding).(map[string]any); ok {
		log.Printf("Binding: %v\n", binding)
		sessionManager.Put(r.Context(), "otp_uri", binding["uri"])
	}
	sessionManager.Put(r.Context(), "friendly_name", *resp.FriendlyName)
	sessionManager.Put(r.Context(), "sid", *resp.Sid)
	sessionManager.Put(r.Context(), "url", *resp.Url)

	http.Redirect(w, r, "/challenge", http.StatusSeeOther)
}

// This renders the form with the QR code that the user needs to scan to verify
// themselves.
//
// After scanning the QR code and retrieving the TOTP code, they can enter it
// into the form and click "Verify" to finish the verification process.
func (app *Application) displayVerifyUserForm(w http.ResponseWriter, r *http.Request) {
	verifyTmpl, err := template.ParseFiles("./templates/verify-user.tmpl")
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	uri := sessionManager.GetString(r.Context(), "otp_uri")
	if uri == "" {
		log.Println("An empty was retrieved from the session")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err = qrcode.WriteFile(uri, qrcode.Medium, 512, "assets/qrcodes/qr.png")
	if err != nil {
		log.Printf("Could not create QR code, because %s", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	type VerifyOtpTemplateData struct {
		Seed string
	}
	err = verifyTmpl.Execute(w, VerifyOtpTemplateData{
		Seed: sessionManager.GetString(r.Context(), "seed"),
	})
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// This processes the verify user form. It redirects the user to the
// showQRCodeForm on success.
func (app *Application) processVerifyUserForm(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Printf("Unable to retrieve form data, because %v\n", err)
		http.Error(w, "An OTP code was not supplied", http.StatusBadRequest)
		return
	}

	code := r.Form.Get("code")
	if code == "" {
		http.Error(w, "The code was not available", http.StatusBadRequest)
		return
	}

	params := &verify.UpdateFactorParams{}
	params.SetAuthPayload(code)

	resp, err := app.twilioRestClient.VerifyV2.UpdateFactor(
		app.verifyServiceSid,
		sessionManager.GetString(r.Context(), "seed"),
		sessionManager.GetString(r.Context(), "sid"),
		params,
	)
	if err != nil {
		log.Println(err.Error())
		os.Exit(1)
	} else {
		if resp.Status != nil {
			log.Println(resp.Status)
		} else {
			log.Println(resp.Status)
		}
	}

	if resp != nil && resp.Status != nil && *resp.Status == "verified" {
		log.Print("Factor setup complete.")
		sessionManager.Put(r.Context(), "flash", "Factor setup complete.")
		http.Redirect(w, r, "/token", http.StatusSeeOther)
	} else {
		log.Print("Factor setup incomplete.")
		http.Redirect(w, r, "/challenge", http.StatusSeeOther)
	}
}

// This renders a form with a QR code and a field for the the TOTP to be
// entered.
func (app *Application) showQRCodeForm(w http.ResponseWriter, r *http.Request) {
	signInTmpl, err := template.ParseFiles("./templates/enter-code.tmpl")
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	type TemplateData struct {
		AlertType, FriendlyName, Identity, Message, Seed string
	}

	message := sessionManager.PopString(r.Context(), "flash")
	data := TemplateData{
		FriendlyName: sessionManager.GetString(r.Context(), "friendly_name"),
		Identity:     sessionManager.GetString(r.Context(), "sid"),
		Message:      message,
		Seed:         sessionManager.GetString(r.Context(), "seed"),
	}

	if message == "Verification success." || message == "Factor setup complete." {
		data.AlertType = "success"
	} else {
		data.AlertType = "error"
	}

	err = signInTmpl.Execute(w, data)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// This processes the showQRCodeForm form and verifies the TOTP submitted in
// the showQRCodeForm.
func (app *Application) processQRCodeForm(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Fatal(err)
	}
	code := r.Form.Get("code")

	params := &verify.CreateChallengeParams{}
	params.SetAuthPayload(code)
	params.SetFactorSid(sessionManager.GetString(r.Context(), "sid"))

	resp, err := app.twilioRestClient.VerifyV2.CreateChallenge(
		app.verifyServiceSid,
		sessionManager.GetString(r.Context(), "seed"),
		params,
	)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if resp.Status != nil {
		if *resp.Status == "approved" {
			sessionManager.Put(r.Context(), "flash", "Verification success.")
		} else {
			sessionManager.Put(r.Context(), "flash", "Verification failed.")
		}
		http.Redirect(w, r, "/token", http.StatusSeeOther)
	}
}
