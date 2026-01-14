module github.com/obot-platform/tools/auth-providers-common

go 1.25.5

// Use obot-platform's fork of oauth2-proxy with custom enhancements (branch: v7.13.0-obot1)
replace github.com/oauth2-proxy/oauth2-proxy/v7 => github.com/obot-platform/oauth2-proxy/v7 v7.0.0-20251217200841-ef3dff8f6dc9

require (
	github.com/lib/pq v1.10.9
	github.com/oauth2-proxy/oauth2-proxy/v7 v7.8.1
)

require (
	github.com/a8m/envsubst v1.4.3 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/bitly/go-simplejson v0.5.1 // indirect
	github.com/coreos/go-oidc/v3 v3.14.1 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/ghodss/yaml v1.0.1-0.20220118164431-d8423dcdf344 // indirect
	github.com/go-jose/go-jose/v4 v4.1.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/sagikazarmark/locafero v0.9.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.14.0 // indirect
	github.com/spf13/cast v1.9.2 // indirect
	github.com/spf13/pflag v1.0.7 // indirect
	github.com/spf13/viper v1.20.1 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apimachinery v0.33.3 // indirect
)
