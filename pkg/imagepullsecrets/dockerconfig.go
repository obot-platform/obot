package imagepullsecrets

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type dockerConfig struct {
	Auths map[string]dockerAuth `json:"auths"`
}

type dockerAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"`
}

func buildDockerConfigJSON(auths map[string]dockerAuth) ([]byte, error) {
	return json.Marshal(dockerConfig{
		Auths: auths,
	})
}

func BuildDockerConfigJSON(server, username, password string) ([]byte, error) {
	server, err := NormalizeRegistryServer(server)
	if err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if password == "" {
		return nil, fmt.Errorf("password is required")
	}

	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	return buildDockerConfigJSON(map[string]dockerAuth{
		server: {
			Username: username,
			Password: password,
			Auth:     auth,
		},
	})
}
