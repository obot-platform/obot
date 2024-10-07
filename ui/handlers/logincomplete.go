package handlers

import (
	"net/http"

	"github.com/gptscript-ai/otto/ui/pages"
)

func LoginComplete(rw http.ResponseWriter, req *http.Request) error {
	return Render(rw, req, pages.LoginComplete())
}
