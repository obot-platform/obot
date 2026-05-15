package imagepullsecrets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const registryTestTimeout = 20 * time.Second

var (
	imageRepositoryPattern = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*(\/[a-z0-9]+([._-][a-z0-9]+)*)*$`)
	imageTagPattern        = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
)

type RegistryTestResult struct {
	Success bool
	Message string
}

type registryChallenge struct {
	scheme string
	params map[string]string
}

type imageReference struct {
	Registry   string
	Repository string
	Reference  string
}

func TestBasicRegistryCredentials(ctx context.Context, server, username, password, image string) (RegistryTestResult, error) {
	client := &http.Client{Timeout: registryTestTimeout}
	return testBasicRegistryCredentials(ctx, client, server, username, password, image)
}

func TestDockerConfigJSONCredentials(ctx context.Context, dockerConfigJSON []byte, image string) (RegistryTestResult, error) {
	client := &http.Client{Timeout: registryTestTimeout}
	return testDockerConfigJSONCredentials(ctx, client, dockerConfigJSON, image)
}

func testDockerConfigJSONCredentials(ctx context.Context, client *http.Client, dockerConfigJSON []byte, image string) (RegistryTestResult, error) {
	var config dockerConfig
	if err := json.Unmarshal(dockerConfigJSON, &config); err != nil {
		return RegistryTestResult{}, fmt.Errorf("image pull secret contains invalid docker config JSON")
	}
	if len(config.Auths) == 0 {
		return RegistryTestResult{}, fmt.Errorf("image pull secret does not contain any docker auth entries")
	}

	ref, err := parseExplicitImageReference(image)
	if err != nil {
		return RegistryTestResult{}, err
	}

	for server, auth := range config.Auths {
		normalizedServer, err := normalizeDockerConfigAuthServer(server)
		if err != nil {
			continue
		}
		if !sameRegistry(normalizedServer, ref.Registry) {
			continue
		}

		username, password, err := dockerAuthCredentials(auth)
		if err != nil {
			return RegistryTestResult{}, err
		}
		result, err := testBasicRegistryCredentials(ctx, client, normalizedServer, username, password, image)
		if err != nil {
			return RegistryTestResult{}, err
		}
		result.Message = "image pull secret can access the image manifest"
		return result, nil
	}

	return RegistryTestResult{}, fmt.Errorf("image pull secret does not contain auth for registry %q", ref.Registry)
}

// testBasicRegistryCredentials validates the stored basic credentials against a
// specific image manifest. It first verifies that the credentials can render a
// Docker config JSON secret, then performs the registry-specific network check.
func testBasicRegistryCredentials(ctx context.Context, client *http.Client, server, username, password, image string) (RegistryTestResult, error) {
	if _, err := BuildDockerConfigJSON(server, username, password); err != nil {
		return RegistryTestResult{}, err
	}

	server, err := NormalizeRegistryServer(server)
	if err != nil {
		return RegistryTestResult{}, err
	}
	username = strings.TrimSpace(username)
	image = strings.TrimSpace(image)
	if image == "" {
		return RegistryTestResult{}, fmt.Errorf("image is required")
	}

	ref, err := parseImageReference(image, server)
	if err != nil {
		return RegistryTestResult{}, err
	}
	if !sameRegistry(server, ref.Registry) {
		return RegistryTestResult{}, fmt.Errorf("image registry %q does not match configured registry server %q", ref.Registry, server)
	}
	if err := testManifestAccess(ctx, client, server, ref, username, password); err != nil {
		return RegistryTestResult{}, err
	}
	return RegistryTestResult{
		Success: true,
		Message: "registry credentials can access the image manifest",
	}, nil
}

// parseImageReference converts a Docker image reference into the registry,
// repository, and tag or digest pieces needed by the Registry HTTP API.
func parseImageReference(image, defaultRegistry string) (imageReference, error) {
	defaultRegistry, err := NormalizeRegistryServer(defaultRegistry)
	if err != nil {
		return imageReference{}, err
	}

	image = strings.TrimSpace(image)
	if image == "" {
		return imageReference{}, fmt.Errorf("image is required")
	}
	if strings.ContainsAny(image, " \t\r\n") {
		return imageReference{}, fmt.Errorf("image reference must not contain whitespace")
	}
	if strings.Contains(image, "://") {
		return imageReference{}, fmt.Errorf("image reference must not include a URL scheme")
	}

	// Split the image into name and reference. Digests take precedence over tags,
	// and tags are only recognized after the last slash so registry ports are not
	// mistaken for image tags.
	reference := "latest"
	remainder := image
	if before, after, ok := strings.Cut(remainder, "@"); ok {
		if after == "" {
			return imageReference{}, fmt.Errorf("image digest is required")
		}
		reference = after
		remainder = before
	} else if tagIndex := strings.LastIndex(remainder, ":"); tagIndex > strings.LastIndex(remainder, "/") {
		reference = remainder[tagIndex+1:]
		remainder = remainder[:tagIndex]
		if !imageTagPattern.MatchString(reference) {
			return imageReference{}, fmt.Errorf("image tag is invalid")
		}
	}
	if remainder == "" {
		return imageReference{}, fmt.Errorf("image repository is required")
	}

	// Docker image references only include an explicit registry when the first
	// path component is registry-like: it contains a dot, contains a port, or is
	// localhost. Otherwise the configured registry is used.
	registry := defaultRegistry
	repository := remainder
	if first, rest, ok := strings.Cut(remainder, "/"); ok && isRegistryComponent(first) {
		registry, err = NormalizeRegistryServer(first)
		if err != nil {
			return imageReference{}, fmt.Errorf("invalid image registry: %w", err)
		}
		repository = rest
	}

	if repository == "" {
		return imageReference{}, fmt.Errorf("image repository is required")
	}
	// Docker Hub official images use the implicit library/ namespace.
	if isDockerHubRegistry(registry) && !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}
	if !imageRepositoryPattern.MatchString(repository) {
		return imageReference{}, fmt.Errorf("image repository must be lowercase and contain only valid Docker repository characters")
	}

	return imageReference{
		Registry:   registry,
		Repository: repository,
		Reference:  reference,
	}, nil
}

func parseExplicitImageReference(image string) (imageReference, error) {
	image = strings.TrimSpace(image)
	namePart := image
	if before, _, ok := strings.Cut(namePart, "@"); ok {
		namePart = before
	} else if tagIndex := strings.LastIndex(namePart, ":"); tagIndex > strings.LastIndex(namePart, "/") {
		namePart = namePart[:tagIndex]
	}

	first, _, ok := strings.Cut(namePart, "/")
	if !ok || !isRegistryComponent(first) {
		// Docker config tests must select an auth entry before validating access.
		// Require an explicit registry so multi-auth secrets, like ECR, are unambiguous.
		return imageReference{}, fmt.Errorf("image reference must include an explicit registry")
	}

	registry, err := NormalizeRegistryServer(first)
	if err != nil {
		return imageReference{}, fmt.Errorf("invalid image registry: %w", err)
	}
	return parseImageReference(image, registry)
}

// testManifestAccess requests the image manifest once anonymously, then follows
// the registry's authentication challenge when credentials are required.
func testManifestAccess(ctx context.Context, client *http.Client, server string, ref imageReference, username, password string) error {
	manifestPath := "/v2/" + escapeRepositoryPath(ref.Repository) + "/manifests/" + url.PathEscape(ref.Reference)
	endpoint := registryURL(server, manifestPath)
	scope := "repository:" + ref.Repository + ":pull"

	resp, err := registryRequest(ctx, client, http.MethodGet, endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return authenticateAndRetry(ctx, client, resp.Header.Values("WWW-Authenticate"), http.MethodGet, endpoint, server, scope, username, password)
	case http.StatusForbidden:
		return fmt.Errorf("credentials do not have pull access to %q", ref.Repository)
	case http.StatusNotFound:
		return fmt.Errorf("image manifest was not found or credentials do not have pull access")
	default:
		return statusError("image manifest check failed", resp.StatusCode)
	}
}

// authenticateAndRetry handles Basic and Bearer challenges from a registry and
// retries the original request with the matching Authorization header.
func authenticateAndRetry(ctx context.Context, client *http.Client, headers []string, method, endpoint, registryServer, scope, username, password string) error {
	challenges := parseAuthChallenges(headers)
	if len(challenges) == 0 {
		return fmt.Errorf("registry returned an authentication challenge Obot could not interpret")
	}

	var unsupported []string
	for _, challenge := range challenges {
		switch challenge.scheme {
		case "basic":
			resp, err := registryRequestWithAuth(ctx, client, method, endpoint, manifestAcceptHeader(), basicAuth(username, password))
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				return fmt.Errorf("registry rejected the credentials with HTTP %d", resp.StatusCode)
			}
			return statusError("authenticated registry check failed", resp.StatusCode)
		case "bearer":
			token, err := requestBearerToken(ctx, client, challenge, registryServer, scope, username, password)
			if err != nil {
				return err
			}
			resp, err := registryRequestWithAuth(ctx, client, method, endpoint, manifestAcceptHeader(), "Bearer "+token)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				return fmt.Errorf("registry rejected the credentials with HTTP %d", resp.StatusCode)
			}
			if resp.StatusCode == http.StatusNotFound && scope != "" {
				return fmt.Errorf("image manifest was not found or credentials do not have pull access")
			}
			return statusError("authenticated registry check failed", resp.StatusCode)
		default:
			unsupported = append(unsupported, challenge.scheme)
		}
	}

	message := "registry did not offer Basic or Bearer authentication"
	if len(unsupported) > 0 {
		message = fmt.Sprintf("registry offered unsupported authentication scheme %q", strings.Join(unsupported, ", "))
	}
	return fmt.Errorf("%s", message)
}

// requestBearerToken exchanges the user's basic credentials for a registry
// Bearer token scoped to the repository pull operation.
func requestBearerToken(ctx context.Context, client *http.Client, challenge registryChallenge, registryServer, scope, username, password string) (string, error) {
	realm := challenge.params["realm"]
	if realm == "" {
		return "", fmt.Errorf("registry Bearer challenge did not include a token realm")
	}

	tokenURL, err := url.Parse(realm)
	if err != nil || tokenURL.Scheme == "" || tokenURL.Host == "" {
		return "", fmt.Errorf("registry Bearer challenge included an invalid token realm")
	}
	if tokenURL.Scheme != "https" {
		return "", fmt.Errorf("registry Bearer challenge token realm must use https")
	}
	realmRegistry, err := NormalizeRegistryServer(tokenURL.Host)
	if err != nil {
		return "", fmt.Errorf("registry Bearer challenge included an invalid token realm")
	}
	if !sameRegistry(registryServer, realmRegistry) {
		return "", fmt.Errorf("registry Bearer challenge token realm does not match the configured registry")
	}
	query := tokenURL.Query()
	if service := challenge.params["service"]; service != "" && query.Get("service") == "" {
		query.Set("service", service)
	}
	if scope != "" {
		query.Set("scope", scope)
	} else if challengeScope := challenge.params["scope"]; challengeScope != "" && query.Get("scope") == "" {
		query.Set("scope", challengeScope)
	}
	tokenURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL.String(), nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(username, password)
	resp, err := client.Do(req)
	if err != nil {
		return "", sanitizeNetworkError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("registry token service rejected the credentials with HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", statusError("registry token request failed", resp.StatusCode)
	}

	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", fmt.Errorf("registry token service returned an invalid response")
	}
	token := payload.Token
	if token == "" {
		token = payload.AccessToken
	}
	if token == "" {
		return "", fmt.Errorf("registry token service did not return a token")
	}
	return token, nil
}

func registryRequest(ctx context.Context, client *http.Client, method, endpoint string) (*http.Response, error) {
	return registryRequestWithAuth(ctx, client, method, endpoint, manifestAcceptHeader(), "")
}

// registryRequestWithAuth builds a registry HTTP request with the shared headers
// this test path needs, including the optional Authorization header.
func registryRequestWithAuth(ctx context.Context, client *http.Client, method, endpoint, accept, authHeader string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "obot-image-pull-secret-test")
	req.Header.Set("Accept", accept)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, sanitizeNetworkError(err)
	}
	return resp, nil
}

// parseAuthChallenges parses WWW-Authenticate headers into normalized schemes
// and parameter maps. Multiple header values are preserved as separate choices.
func parseAuthChallenges(headers []string) []registryChallenge {
	var result []registryChallenge
	for _, header := range headers {
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}
		scheme, params, ok := strings.Cut(header, " ")
		if !ok {
			result = append(result, registryChallenge{scheme: strings.ToLower(header), params: map[string]string{}})
			continue
		}
		// Challenges look like `Bearer key="value",...` or `Basic realm="..."`.
		// Keep the auth scheme separate from its comma-delimited parameters.
		result = append(result, registryChallenge{
			scheme: strings.ToLower(strings.TrimSpace(scheme)),
			params: parseAuthParams(params),
		})
	}
	return result
}

// parseAuthParams parses a comma-delimited authentication parameter list while
// preserving quoted values that can contain commas or escaped characters.
func parseAuthParams(input string) map[string]string {
	result := map[string]string{}
	for len(input) > 0 {
		input = strings.TrimLeft(input, " \t,")
		if input == "" {
			break
		}
		keyEnd := strings.IndexByte(input, '=')
		if keyEnd <= 0 {
			break
		}
		key := strings.ToLower(strings.TrimSpace(input[:keyEnd]))
		input = strings.TrimLeft(input[keyEnd+1:], " \t")

		var value string
		if strings.HasPrefix(input, `"`) {
			// Quoted values may contain escaped quotes or commas, so scan until the
			// matching unescaped quote instead of splitting on every comma.
			var builder strings.Builder
			escaped := false
			foundClosingQuote := false
			input = input[1:]
			for i, r := range input {
				if escaped {
					builder.WriteRune(r)
					escaped = false
					continue
				}
				if r == '\\' {
					escaped = true
					continue
				}
				if r == '"' {
					value = builder.String()
					input = input[i+1:]
					foundClosingQuote = true
					break
				}
				builder.WriteRune(r)
			}
			if !foundClosingQuote {
				value = builder.String()
				input = ""
			}
		} else {
			valueEnd := strings.IndexByte(input, ',')
			if valueEnd < 0 {
				value = strings.TrimSpace(input)
				input = ""
			} else {
				value = strings.TrimSpace(input[:valueEnd])
				input = input[valueEnd+1:]
			}
		}
		if key != "" {
			result[key] = value
		}
	}
	return result
}

func registryURL(server, path string) string {
	u := url.URL{
		Scheme: "https",
		Host:   registryAPIHost(server),
		Path:   path,
	}
	return u.String()
}

func registryAPIHost(server string) string {
	if isDockerHubRegistry(server) {
		return "registry-1.docker.io"
	}
	return server
}

func normalizeDockerConfigAuthServer(server string) (string, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return "", fmt.Errorf("docker auth server is empty")
	}
	if strings.Contains(server, "://") {
		parsed, err := url.Parse(server)
		if err != nil {
			return "", err
		}
		server = parsed.Host
	} else if before, _, ok := strings.Cut(server, "/"); ok {
		server = before
	}
	return NormalizeRegistryServer(server)
}

func dockerAuthCredentials(auth dockerAuth) (string, string, error) {
	if auth.Username != "" && auth.Password != "" {
		return auth.Username, auth.Password, nil
	}
	if auth.Auth == "" {
		return "", "", fmt.Errorf("docker auth entry is missing credentials")
	}
	decoded, err := base64.StdEncoding.DecodeString(auth.Auth)
	if err != nil {
		return "", "", fmt.Errorf("docker auth entry contains invalid credentials")
	}
	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok || username == "" || password == "" {
		return "", "", fmt.Errorf("docker auth entry contains invalid credentials")
	}
	return username, password, nil
}

func isRegistryComponent(component string) bool {
	return strings.Contains(component, ".") || strings.Contains(component, ":") || component == "localhost"
}

func isDockerHubRegistry(server string) bool {
	switch server {
	case "docker.io", "index.docker.io", "registry-1.docker.io":
		return true
	default:
		return false
	}
}

func sameRegistry(a, b string) bool {
	if isDockerHubRegistry(a) && isDockerHubRegistry(b) {
		return true
	}
	return strings.EqualFold(a, b)
}

func escapeRepositoryPath(repository string) string {
	parts := strings.Split(repository, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func manifestAcceptHeader() string {
	return strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.docker.distribution.manifest.v1+json",
		"*/*",
	}, ", ")
}

func basicAuth(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

func sanitizeNetworkError(err error) error {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("registry request timed out")
	}
	return fmt.Errorf("registry request failed")
}

func statusError(prefix string, statusCode int) error {
	return fmt.Errorf("%s with HTTP %d", prefix, statusCode)
}
