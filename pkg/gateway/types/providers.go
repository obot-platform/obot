package types

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type LLMProvider struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug" gorm:"unique;index"`
	BaseURL   string    `json:"baseURL"`
	Token     string    `json:"token"`
	Disabled  bool      `json:"disabled"`
}

func (lp *LLMProvider) Validate() error {
	var errs []error
	if lp.Name == "" {
		errs = append(errs, fmt.Errorf("provider name is required"))
	}
	if lp.BaseURL == "" {
		errs = append(errs, fmt.Errorf("provider base URL is required"))
	}
	if lp.Token == "" {
		errs = append(errs, fmt.Errorf("provider token is required"))
	}

	if lp.Slug == "" {
		lp.Slug = url.PathEscape(strings.ReplaceAll(strings.ToLower(lp.Name), " ", "-"))
	} else {
		lp.Slug = url.PathEscape(lp.Slug)
	}

	return errors.Join(errs...)
}

func (lp *LLMProvider) RequestBaseURL(serverBase string) string {
	return fmt.Sprintf("%s/llm/%s", serverBase, lp.Slug)
}

func (lp *LLMProvider) URL() string {
	return fmt.Sprintf("%s/llm/%s", lp.BaseURL, lp.Slug)
}
