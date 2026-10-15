package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Zetshin/movie-reviews/internal/models"
	"github.com/Zetshin/movie-reviews/internal/validator"
)

type forgotPasswordForm struct {
	Email               string `form:"email"`
	validator.Validator `form:"-"`
}

type resetPasswordForm struct {
	Token               string `form:"token"`
	Password            string `form:"password"`
	ConfirmPassword     string `form:"confirm_password"`
	validator.Validator `form:"-"`
}

func (app *application) forgotPassword(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = forgotPasswordForm{}
	app.render(w, r, http.StatusOK, "forgot-password.tmpl", data)
}

func (app *application) forgotPasswordPost(w http.ResponseWriter, r *http.Request) {
	var form forgotPasswordForm
	if err := app.decodePostForm(r, &form); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	form.Email = strings.TrimSpace(form.Email)
	form.CheckField(validator.NotBlank(form.Email), "email", "This field cannot be blank")
	form.CheckField(validator.MaxChars(form.Email, 254), "email", "This field must be 254 characters or fewer")
	form.CheckField(validator.Matches(form.Email, validator.EmailRX), "email", "This field must be a valid email address")
	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Form = form
		app.render(w, r, http.StatusUnprocessableEntity, "forgot-password.tmpl", data)
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		app.serverError(w, r, err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	found, err := app.users.CreatePasswordReset(form.Email, token, time.Now().UTC().Add(30*time.Minute))
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if found && app.passwordMailer != nil {
		resetURL := strings.TrimRight(app.config.publicBaseURL, "/") + "/user/reset-password?token=" + url.QueryEscape(token)
		email := form.Email
		go func() {
			if err := app.passwordMailer.SendPasswordReset(email, resetURL); err != nil {
				app.logger.Error("could not send password reset email", "error", err)
			}
		}()
	}
	app.sessionManager.Put(r.Context(), "flash", "If that email is registered, a reset link has been sent.")
	http.Redirect(w, r, "/user/login", http.StatusSeeOther)
}

func (app *application) resetPassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	token := r.URL.Query().Get("token")
	valid, err := app.users.PasswordResetTokenValid(token)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data := app.newTemplateData(r)
	form := resetPasswordForm{Token: token}
	if !valid {
		form.AddNonFieldError("This reset link is invalid or has expired.")
	}
	data.Form = form
	app.render(w, r, http.StatusOK, "reset-password.tmpl", data)
}

func (app *application) resetPasswordPost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	var form resetPasswordForm
	if err := app.decodePostForm(r, &form); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	form.CheckField(validator.NotBlank(form.Password), "password", "This field cannot be blank")
	form.CheckField(validator.MinChars(form.Password, 8), "password", "This field must be at least 8 characters long")
	form.CheckField(validator.MaxBytes(form.Password, 72), "password", "This field must not be more than 72 bytes long")
	form.CheckField(form.Password == form.ConfirmPassword, "confirm_password", "Passwords do not match")
	valid, err := app.users.PasswordResetTokenValid(form.Token)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !valid {
		form.AddNonFieldError("This reset link is invalid or has expired.")
	}
	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Form = form
		app.render(w, r, http.StatusUnprocessableEntity, "reset-password.tmpl", data)
		return
	}
	if err := app.users.ResetPassword(form.Token, form.Password); err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			form.AddNonFieldError("This reset link is invalid or has expired.")
			data := app.newTemplateData(r)
			data.Form = form
			app.render(w, r, http.StatusUnprocessableEntity, "reset-password.tmpl", data)
			return
		}
		app.serverError(w, r, err)
		return
	}
	_ = app.sessionManager.RenewToken(r.Context())
	app.sessionManager.Remove(r.Context(), "authenticatedUserID")
	app.sessionManager.Remove(r.Context(), "authenticatedAuthVersion")
	app.sessionManager.Put(r.Context(), "flash", "Your password has been reset. Please log in.")
	http.Redirect(w, r, "/user/login", http.StatusSeeOther)
}
