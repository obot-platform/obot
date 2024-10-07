package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gptscript-ai/otto/apiclient"
	"github.com/gptscript-ai/otto/pkg/proxy"
	"github.com/gptscript-ai/otto/ui/router"
)

func main() {
	emailDomains := os.Getenv("OTTO_SERVER_AUTH_EMAIL_DOMAINS")
	if emailDomains == "" {
		emailDomains = "*"
	}
	authConfig := proxy.Config{
		AuthBaseURI:        "http://localhost:8081",
		AuthCookieSecret:   os.Getenv("OTTO_SERVER_AUTH_COOKIE_SECRET"),
		AuthEmailDomains:   emailDomains,
		AuthAdminEmails:    strings.Split(os.Getenv("OTTO_SERVER_AUTH_ADMIN_EMAILS"), ","),
		GoogleClientID:     os.Getenv("OTTO_SERVER_GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("OTTO_SERVER_GOOGLE_CLIENT_SECRET"),
	}
	client := &apiclient.Client{
		BaseURL: "http://localhost:8080",
		Token:   os.Getenv("OTTO_TOKEN"),
	}

	middleware := fakeMiddleware
	if authConfig.GoogleClientID != "" && authConfig.GoogleClientSecret != "" {
		oauth2Proxy, err := proxy.New(1, authConfig)
		if err != nil {
			log.Fatal(err)
		}
		middleware = oauth2Proxy.Wrap
	}
	handler := middleware(router.Init(client, true))
	log.Println("Starting server on :8081")
	err := http.ListenAndServe(":8081", handler)
	if err != nil {
		log.Fatal(err)
	}
}

func fakeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		next.ServeHTTP(rw, req)
	})
}
