package handler

import (
	"aiac-service/internal/lib"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func MailerDummy(r chi.Router, mailer *lib.Mailer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := mailer.Send(
			r.Context(),
			"kanzoonrekza@gmail.com",
			"Test Email",
			"This is a dummy test email.",
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("email sent"))
	}
}
