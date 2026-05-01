package imagepullsecrets

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/adhocore/gronx"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
)

var (
	ecrRoleARNPattern = regexp.MustCompile(`^arn:(aws|aws-cn|aws-us-gov):iam::[0-9]{12}:role/[A-Za-z0-9+=,.@_/-]+$`)
	awsRegionPattern  = regexp.MustCompile(`^[a-z]{2}(-gov)?-[a-z0-9-]+-[0-9]+$`)
)

func NormalizeRegistryServer(server string) (string, error) {
	original := strings.TrimSpace(server)
	if original == "" {
		return "", fmt.Errorf("registry server is required")
	}
	if strings.ContainsAny(original, " \t\r\n") {
		return "", fmt.Errorf("registry server must not contain whitespace")
	}

	parseValue := original
	hasScheme := strings.Contains(original, "://")
	if !hasScheme {
		if strings.Contains(original, "/") {
			return "", fmt.Errorf("registry server must not include a URL path")
		}
		parseValue = "https://" + original
	}

	parsed, err := url.Parse(parseValue)
	if err != nil {
		return "", fmt.Errorf("invalid registry server: %w", err)
	}
	if hasScheme && parsed.Scheme != "https" {
		return "", fmt.Errorf("registry server scheme must be https")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("registry server must not include user info")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("registry server must not include a URL path")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("registry server host is required")
	}

	host, port, err := splitHostPort(parsed.Host)
	if err != nil {
		return "", err
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if err := validateRegistryHost(host); err != nil {
		return "", err
	}
	if port == "" {
		if strings.HasSuffix(parsed.Host, ":") {
			return "", fmt.Errorf("registry server port is required")
		}
		if strings.Contains(host, ":") {
			return "[" + host + "]", nil
		}
		return host, nil
	}

	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", fmt.Errorf("registry server port must be a number between 1 and 65535")
	}
	return net.JoinHostPort(host, port), nil
}

func ValidateSecretName(secretName string) error {
	secretName = strings.TrimSpace(secretName)
	if secretName == "" {
		return fmt.Errorf("secretName is required")
	}
	if errs := kvalidation.IsDNS1123Label(secretName); len(errs) > 0 {
		return fmt.Errorf("secretName must be a valid Kubernetes DNS label: %s", strings.Join(errs, "; "))
	}
	return nil
}

func ValidateSpec(spec v1.ImagePullSecretSpec) (v1.ImagePullSecretSpec, error) {
	spec.SecretName = strings.TrimSpace(spec.SecretName)
	if err := ValidateSecretName(spec.SecretName); err != nil {
		return spec, err
	}

	switch spec.Type {
	case v1.ImagePullSecretTypeBasic:
		basic, err := ValidateBasicSpec(spec.Basic)
		if err != nil {
			return spec, err
		}
		spec.Basic = basic
		spec.ECR = nil
	case v1.ImagePullSecretTypeECR:
		ecr, err := ValidateECRSpec(spec.ECR)
		if err != nil {
			return spec, err
		}
		spec.Basic = nil
		spec.ECR = ecr
	default:
		return spec, fmt.Errorf("type must be one of %q or %q", v1.ImagePullSecretTypeBasic, v1.ImagePullSecretTypeECR)
	}

	return spec, nil
}

func ValidateBasicSpec(spec *v1.BasicImagePullSecretSpec) (*v1.BasicImagePullSecretSpec, error) {
	if spec == nil {
		return nil, fmt.Errorf("basic configuration is required")
	}

	result := *spec
	server, err := NormalizeRegistryServer(result.Server)
	if err != nil {
		return nil, err
	}
	result.Server = server
	result.Username = strings.TrimSpace(result.Username)
	if result.Username == "" {
		return nil, fmt.Errorf("username is required")
	}

	return &result, nil
}

func ValidateECRSpec(spec *v1.ECRImagePullSecretSpec) (*v1.ECRImagePullSecretSpec, error) {
	if spec == nil {
		return nil, fmt.Errorf("ecr configuration is required")
	}

	result := *spec
	result.RoleARN = strings.TrimSpace(result.RoleARN)
	if !ecrRoleARNPattern.MatchString(result.RoleARN) {
		return nil, fmt.Errorf("roleARN must be a valid AWS IAM role ARN")
	}

	result.Region = strings.TrimSpace(result.Region)
	if !awsRegionPattern.MatchString(result.Region) {
		return nil, fmt.Errorf("region must be a valid AWS region")
	}

	issuerURL, err := NormalizeIssuerURL(result.IssuerURL)
	if err != nil {
		return nil, err
	}
	result.IssuerURL = issuerURL

	result.Audience = strings.TrimSpace(result.Audience)
	if result.Audience == "" {
		result.Audience = DefaultECRAudience
	}

	result.RefreshSchedule = strings.TrimSpace(result.RefreshSchedule)
	if result.RefreshSchedule == "" {
		result.RefreshSchedule = DefaultECRRefreshSchedule
	}
	if !gronx.IsValid(result.RefreshSchedule) {
		return nil, fmt.Errorf("refreshSchedule must be a valid cron expression")
	}

	return &result, nil
}

func NormalizeIssuerURL(issuerURL string) (string, error) {
	issuerURL = strings.TrimSpace(issuerURL)
	if issuerURL == "" {
		return "", nil
	}

	parsed, err := url.Parse(issuerURL)
	if err != nil {
		return "", fmt.Errorf("invalid issuerURL: %w", err)
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("issuerURL must use https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("issuerURL host is required")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("issuerURL must not include query or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func splitHostPort(hostPort string) (string, string, error) {
	host, port, err := net.SplitHostPort(hostPort)
	if err == nil {
		return host, port, nil
	}
	if strings.Contains(err.Error(), "missing port in address") {
		return strings.TrimPrefix(strings.TrimSuffix(hostPort, "]"), "["), "", nil
	}
	return "", "", fmt.Errorf("invalid registry server host")
}

func validateRegistryHost(host string) error {
	if host == "" {
		return fmt.Errorf("registry server host is required")
	}
	if ip := net.ParseIP(host); ip != nil {
		return nil
	}
	if host == "localhost" {
		return nil
	}
	if errs := kvalidation.IsDNS1123Subdomain(host); len(errs) > 0 {
		return fmt.Errorf("registry server host must be a valid DNS name or IP address: %s", strings.Join(errs, "; "))
	}
	return nil
}
