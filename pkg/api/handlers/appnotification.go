package handlers

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AppNotificationHandler struct{}

func NewAppNotificationHandler() *AppNotificationHandler {
	return &AppNotificationHandler{}
}

func (h *AppNotificationHandler) Get(req api.Context) error {
	var notification v1.AppNotification
	err := req.Storage.Get(req.Context(), client.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.AppNotificationName,
	}, &notification)
	if apierrors.IsNotFound(err) {
		// Return empty notification if not yet configured
		return req.Write(types.AppNotification{})
	}
	if err != nil {
		return err
	}

	converted := convertAppNotification(notification)
	return req.Write(converted)
}

func (h *AppNotificationHandler) Update(req api.Context) error {
	var input types.AppNotification
	if err := req.Read(&input); err != nil {
		return err
	}

	if err := validateBanner(input.Banner); err != nil {
		return err
	}

	var notification v1.AppNotification
	err := req.Get(&notification, system.AppNotificationName)
	if apierrors.IsNotFound(err) {
		// Create new notification
		notification = v1.AppNotification{
			ObjectMeta: metav1.ObjectMeta{
				Name:      system.AppNotificationName,
				Namespace: req.Namespace(),
			},
			Spec: v1.AppNotificationSpec{
				Banner:  input.Banner,
				Updated: metav1.Now(),
			},
		}

		if err := req.Create(&notification); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// Update existing notification
		notification.Spec.Banner = input.Banner
		notification.Spec.Updated = metav1.Now()

		if err := req.Update(&notification); err != nil {
			return err
		}
	}

	converted := convertAppNotification(notification)
	return req.Write(converted)
}

// Banner text only supports simple inline formatting (bold, italic, strikethrough)
// and HTTP(S) markdown text links; everything else is rejected.
var disallowedBannerPatterns = []*regexp.Regexp{
	regexp.MustCompile("```"),                           // fenced code blocks
	regexp.MustCompile(`!\[[^\]]*]\([^)]*\)`),           // images
	regexp.MustCompile(`(?i)</?[a-z][^>]*>`),            // raw HTML tags
	regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s`),          // headings
	regexp.MustCompile(`(?m)^\s{0,3}>\s`),               // blockquotes
	regexp.MustCompile(`(?m)^\s{0,3}(?:[-*+]|\d+\.)\s`), // lists
	regexp.MustCompile(`(?m)^\s{0,3}(?:[-*_]\s*){3,}$`), // horizontal rules
	regexp.MustCompile(`\[[^\]]+]\[[^\]]*]`),            // reference-style links
	regexp.MustCompile(`(?m)^\s*\|.+\|\s*$`),            // tables
}

var bannerLinkPattern = regexp.MustCompile(`\[([^\]]+)]\(([^)]+)\)`)

const bannerTextValidationError = "banner text only supports simple formatting and HTTP(S) text links (bold, italic, strikethrough, and [text](url))"

func validateBanner(banner types.BannerNotification) error {
	text := strings.TrimSpace(banner.Text)

	if text != "" {
		if err := validateBannerText(text); err != nil {
			return err
		}
	}

	if banner.Enabled && (text == "" || banner.Type == "") {
		return types.NewErrBadRequest("banner text and type are required when the banner is enabled")
	}

	if banner.Type != "" && banner.Type != types.BannerTypeInfo && banner.Type != types.BannerTypeWarning {
		return types.NewErrBadRequest("invalid banner type: %s", banner.Type)
	}

	return nil
}

func validateBannerText(text string) error {
	for _, pattern := range disallowedBannerPatterns {
		if pattern.MatchString(text) {
			return types.NewErrBadRequest("%s", bannerTextValidationError)
		}
	}

	for _, match := range bannerLinkPattern.FindAllStringSubmatch(text, -1) {
		label, href := match[1], match[2]
		if strings.TrimSpace(label) == "" {
			return types.NewErrBadRequest("%s", bannerTextValidationError)
		}

		parsed, err := url.Parse(strings.TrimSpace(href))
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return types.NewErrBadRequest("%s", bannerTextValidationError)
		}
	}

	textWithoutLinks := bannerLinkPattern.ReplaceAllString(text, "$1")
	if strings.ContainsAny(textWithoutLinks, "\\`") {
		return types.NewErrBadRequest("%s", bannerTextValidationError)
	}

	return nil
}

func convertAppNotification(notification v1.AppNotification) types.AppNotification {
	// If Updated was never set (for example, objects created before Updated tracking existed),
	// fall back to the creation timestamp.
	updated := notification.Spec.Updated.Time
	if updated.IsZero() {
		updated = notification.GetCreationTimestamp().Time
	}

	return types.AppNotification{
		Banner:  notification.Spec.Banner,
		Updated: types.NewTime(updated),
	}
}
