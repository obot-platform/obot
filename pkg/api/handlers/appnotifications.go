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

type AppNotificationsHandler struct{}

func NewAppNotificationsHandler() *AppNotificationsHandler {
	return &AppNotificationsHandler{}
}

func (h *AppNotificationsHandler) Get(req api.Context) error {
	var notifications v1.AppNotifications
	err := req.Storage.Get(req.Context(), client.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.AppNotificationsName,
	}, &notifications)

	if apierrors.IsNotFound(err) {
		// Return empty notifications if not yet configured
		return req.Write(types.AppNotifications{})
	}
	if err != nil {
		return err
	}

	converted := convertAppNotifications(notifications)
	return req.Write(converted)
}

func (h *AppNotificationsHandler) Update(req api.Context) error {
	var input types.AppNotifications
	if err := req.Read(&input); err != nil {
		return err
	}

	if err := validateBanner(input.Banner); err != nil {
		return err
	}

	var notifications v1.AppNotifications
	err := req.Get(&notifications, system.AppNotificationsName)

	if apierrors.IsNotFound(err) {
		// Create new notifications
		notifications = v1.AppNotifications{
			ObjectMeta: metav1.ObjectMeta{
				Name:      system.AppNotificationsName,
				Namespace: req.Namespace(),
			},
			Spec: v1.AppNotificationsSpec{
				Banner:         input.Banner,
				ResetDismissed: input.ResetDismissed,
			},
		}

		if err := req.Create(&notifications); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// Update existing notifications
		notifications.Spec.Banner = input.Banner
		notifications.Spec.ResetDismissed = input.ResetDismissed
		notifications.Spec.Updated = metav1.Now()

		if err := req.Update(&notifications); err != nil {
			return err
		}
	}

	converted := convertAppNotifications(notifications)
	return req.Write(converted)
}

// Banner text only supports simple inline formatting (bold, italic, strikethrough, inline code)
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

const bannerTextValidationError = "banner text only supports simple formatting and HTTP(S) text links (bold, italic, strikethrough, inline code, and [text](url))"

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

func convertAppNotifications(notifications v1.AppNotifications) types.AppNotifications {
	// On first creation, no explicit updated time is stored, so it matches the creation time.
	updated := notifications.Spec.Updated.Time
	if updated.IsZero() {
		updated = notifications.GetCreationTimestamp().Time
	}

	return types.AppNotifications{
		Banner:         notifications.Spec.Banner,
		ResetDismissed: notifications.Spec.ResetDismissed,
		Updated:        *types.NewTime(updated),
		Metadata:       MetadataFrom(&notifications),
	}
}
