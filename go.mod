module github.com/obot-platform/obot

go 1.24.2

replace (
	github.com/danielgtaylor/huma/v2 => github.com/gptscript-ai/huma v0.0.0-20250617131016-b2081da6c65b
	github.com/obot-platform/obot/apiclient => ./apiclient
	github.com/obot-platform/obot/logger => ./logger
)

require (
	github.com/adhocore/gronx v1.19.5
	github.com/adrg/xdg v0.5.3
	github.com/aws/aws-sdk-go-v2 v1.36.3
	github.com/aws/aws-sdk-go-v2/config v1.29.12
	github.com/aws/aws-sdk-go-v2/service/s3 v1.78.2
	github.com/dustin/go-humanize v1.0.1
	github.com/fatih/color v1.18.0
	github.com/gen2brain/webp v0.5.4
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/google/uuid v1.6.0
	github.com/gptscript-ai/chat-completion-client v0.0.0-20250224164718-139cb4507b1d
	github.com/gptscript-ai/cmd v0.0.0-20250530150401-bc71fddf8070
	github.com/gptscript-ai/datasets v0.0.0-20241125193827-31ce6c3c682b
	github.com/gptscript-ai/go-gptscript v0.9.6-0.20250617131750-9129819aea51
	github.com/gptscript-ai/gptscript v0.9.6-0.20250623175741-7a49337eef7b
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de
	github.com/mhale/smtpd v0.8.3
	github.com/nanobot-ai/nanobot v0.0.6-0.20250708021654-6ff874bc6980
	github.com/obot-platform/kinm v0.0.0-20250328185534-d9c9de49cc20
	github.com/obot-platform/nah v0.0.0-20250418220644-1b9278409317
	github.com/obot-platform/namegenerator v0.0.0-20241217121223-fc58bdb7dca2
	github.com/obot-platform/obot/apiclient v0.0.0-00010101000000-000000000000
	github.com/obot-platform/obot/logger v0.0.0-20241217130503-4004a5c69f32
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/prometheus/client_golang v1.20.5
	github.com/pterm/pterm v0.12.80
	github.com/rs/cors v1.11.1
	github.com/sendgrid/sendgrid-go v3.16.0+incompatible
	github.com/sethvargo/go-limiter v1.0.0
	github.com/slack-go/slack v0.17.1
	github.com/spf13/cobra v1.9.1
	github.com/stretchr/testify v1.10.0
	github.com/tidwall/gjson v1.18.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.60.0
	go.opentelemetry.io/otel v1.35.0
	go.opentelemetry.io/otel/log v0.11.0
	go.opentelemetry.io/otel/sdk v1.35.0
	go.opentelemetry.io/otel/sdk/log v0.11.0
	go.opentelemetry.io/otel/sdk/metric v1.35.0
	golang.org/x/crypto v0.38.0
	golang.org/x/exp v0.0.0-20250128182459-e0ece0dbea4c
	golang.org/x/mod v0.22.0
	golang.org/x/term v0.32.0
	golang.org/x/text v0.25.0
	google.golang.org/genai v1.0.0
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/driver/postgres v1.6.0
	gorm.io/gorm v1.30.0
	k8s.io/api v0.31.1
	k8s.io/apimachinery v0.31.1
	k8s.io/apiserver v0.31.1
	k8s.io/client-go v0.31.1
	k8s.io/component-base v0.31.1
	k8s.io/gengo/v2 v2.0.0-20240911193312-2b36238f13e9
	k8s.io/kube-openapi v0.0.0-20241009091222-67ed5848f094
	k8s.io/kubectl v0.29.0
	sigs.k8s.io/controller-runtime v0.19.0
	sigs.k8s.io/yaml v1.4.0
)

require (
	atomicgo.dev/cursor v0.2.0 // indirect
	atomicgo.dev/keyboard v0.2.9 // indirect
	atomicgo.dev/schedule v0.1.0 // indirect
	cloud.google.com/go v0.116.0 // indirect
	cloud.google.com/go/auth v0.9.3 // indirect
	cloud.google.com/go/compute/metadata v0.6.0 // indirect
	dario.cat/mergo v1.0.1 // indirect
	github.com/AlecAivazis/survey/v2 v2.3.7 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/BurntSushi/locker v0.0.0-20171006230638-a6e239ea1c69 // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/ProtonMail/go-crypto v1.1.5 // indirect
	github.com/STARRY-S/zip v0.2.1 // indirect
	github.com/alecthomas/chroma/v2 v2.15.0 // indirect
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20190424111038-f61b66f89f4a // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.10 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.65 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.34 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.34 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.34 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.7.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.18.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.25.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.30.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.33.17 // indirect
	github.com/aws/smithy-go v1.22.2 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/bodgit/plumbing v1.3.0 // indirect
	github.com/bodgit/sevenzip v1.6.0 // indirect
	github.com/bodgit/windows v1.0.1 // indirect
	github.com/bombsimon/logrusr/v4 v4.1.0 // indirect
	github.com/bwmarrin/discordgo v0.28.1
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/chai2010/gettext-go v1.0.2 // indirect
	github.com/charmbracelet/colorprofile v0.2.3-0.20250311203215-f60798e515dc // indirect
	github.com/charmbracelet/glamour v0.8.0 // indirect
	github.com/charmbracelet/lipgloss v1.1.0 // indirect
	github.com/charmbracelet/x/ansi v0.8.0 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.13 // indirect
	github.com/charmbracelet/x/exp/golden v0.0.0-20241011142426-46044092ad91 // indirect
	github.com/charmbracelet/x/term v0.2.1 // indirect
	github.com/chzyer/readline v1.5.1 // indirect
	github.com/cloudflare/circl v1.5.0 // indirect
	github.com/containerd/console v1.0.4 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/cyphar/filepath-securejoin v0.4.1 // indirect
	github.com/danielgtaylor/huma/v2 v2.32.1-0.20250509235652-c7ead6f3c67f // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dlclark/regexp2 v1.11.4 // indirect
	github.com/docker/cli v27.5.1+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.8.2 // indirect
	github.com/dop251/goja v0.0.0-20250531102226-cb187b08699c // indirect
	github.com/dsnet/compress v0.0.2-0.20230904184137-39efe44ab707 // indirect
	github.com/ebitengine/purego v0.8.2 // indirect
	github.com/emicklei/go-restful/v3 v3.12.1 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/evanphx/json-patch v5.7.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/exponent-io/jsonpath v0.0.0-20151013193312-d6023ce2651d // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/fxamacker/cbor/v2 v2.8.0 // indirect
	github.com/getkin/kin-openapi v0.132.0 // indirect
	github.com/glebarez/go-sqlite v1.21.2 // indirect
	github.com/glebarez/sqlite v1.11.0 // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.6.2 // indirect
	github.com/go-git/go-git/v5 v5.13.2 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/btree v1.0.1 // indirect
	github.com/google/cel-go v0.20.1 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20240910150728-a0b0bb1d4134 // indirect
	github.com/google/s2a-go v0.1.8 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.4 // indirect
	github.com/gookit/color v1.5.4 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/gptscript-ai/broadcaster v0.0.0-20240625175512-c43682019b86 // indirect
	github.com/gptscript-ai/tui v0.0.0-20250419050840-5e79e16786c9 // indirect
	github.com/gregjones/httpcache v0.0.0-20180305231024-9cad4c3443a7 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.26.1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.6.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jaytaylor/html2text v0.0.0-20230321000545-74c2419ad056 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/klauspost/pgzip v1.2.6 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/lithammer/fuzzysearch v1.1.8 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/mholt/archives v0.1.0 // indirect
	github.com/microcosm-cc/bluemonday v1.0.27 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/spdystream v0.4.0 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/muesli/reflow v0.3.0 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/nwaples/rardecode/v2 v2.1.0 // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/oasdiff/yaml v0.0.0-20250309154309-f31be36b4037 // indirect
	github.com/oasdiff/yaml3 v0.0.0-20250309153720-d2182401db90 // indirect
	github.com/olekukonko/tablewriter v0.0.6-0.20230925090304-df64c4bbad77 // indirect
	github.com/onsi/ginkgo/v2 v2.20.2 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/pjbgf/sha1cd v0.3.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkoukk/tiktoken-go v0.1.7 // indirect
	github.com/pkoukk/tiktoken-go-loader v0.0.2-0.20240522064338-c17e8bc0f699 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.60.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/skeema/knownhosts v1.3.0 // indirect
	github.com/sorairolake/lzip-go v0.3.5 // indirect
	github.com/sourcegraph/go-diff-patch v0.0.0-20240223163233-798fd1e94a8e // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/ssor/bom v0.0.0-20170718123548-6386211fdfcf // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	github.com/tetratelabs/wazero v1.9.0 // indirect
	github.com/therootcompany/xz v1.0.1 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/ulikunitz/xz v0.5.12 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/xlab/treeprint v1.2.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/yuin/goldmark v1.7.8 // indirect
	github.com/yuin/goldmark-emoji v1.0.4 // indirect
	go.etcd.io/etcd/api/v3 v3.5.14 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.14 // indirect
	go.etcd.io/etcd/client/v3 v3.5.14 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.54.0 // indirect
	go.opentelemetry.io/contrib/samplers/probability/consistent v0.29.0
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.11.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.35.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.33.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.27.0
	go.opentelemetry.io/otel/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	go.opentelemetry.io/proto/otlp v1.5.0 // indirect
	go.starlark.net v0.0.0-20230525235612-a134d8f9ddca // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	go4.org v0.0.0-20230225012048-214862532bf5 // indirect
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/oauth2 v0.30.0
	golang.org/x/sync v0.14.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/time v0.7.0 // indirect
	golang.org/x/tools v0.29.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250218202821-56aae31c358a // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250218202821-56aae31c358a // indirect
	google.golang.org/grpc v1.71.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/cli-runtime v0.29.0 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kms v0.31.1 // indirect
	k8s.io/utils v0.0.0-20240921022957-49e7df575cb6 // indirect
	modernc.org/libc v1.22.5 // indirect
	modernc.org/mathutil v1.5.0 // indirect
	modernc.org/memory v1.5.0 // indirect
	modernc.org/sqlite v1.23.1 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.30.3 // indirect
	sigs.k8s.io/controller-tools v0.16.4 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/kustomize/api v0.13.5-0.20230601165947-6ce0bf390ce3 // indirect
	sigs.k8s.io/kustomize/kyaml v0.14.3-0.20230601165947-6ce0bf390ce3 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
)
