#!/bin/bash
set -e -x -o pipefail

BIN_DIR=${BIN_DIR:-./bin}

# Check if TOOL_REGISTRY_REPOS is set and non-empty
if [[ -z "${TOOL_REGISTRY_REPOS}" ]]; then
    echo "Error: TOOL_REGISTRY_REPOS environment variable is not set or is empty."
    exit 1
fi

if [[ -n "${GITHUB_TOKEN}" ]]; then
    set +x
    git config --global url."https://${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/" 2>/dev/null
    set -x
fi

cd $(dirname $0)/..

if [ ! -e obot-tools ]; then
    mkdir obot-tools
fi
cd obot-tools

# Convert TOOL_REGISTRY_REPOS into an array by replacing commas with spaces
read -r -a TOOL_REPOS <<< "${TOOL_REGISTRY_REPOS//,/ }"
REGISTRY_REMAP=()
LOCAL_REGISTRIES=()
OBOT_SERVER_VERSIONS=""

# Iterate over the repositories
for REPO in "${TOOL_REPOS[@]}"; do
    # Extract the repo name (e.g., tools, enterprise-tools)
    REPO_NAME=$(basename "${REPO}")
    HASH=""
    # If there is a hash in the repo name, then extract it
    if [[ "${REPO_NAME}" == *"@"* ]]; then
      # Extract the part after "@"
      HASH="${REPO_NAME#*@}"
      REPO_NAME=${REPO_NAME%@*}
      REPO=${REPO%@*}
	fi
    REPO_DIR="obot-tools/${REPO_NAME}"

    # Clone the repository into the target directory
    echo "Cloning ${REPO} into ${REPO_DIR}..."
    if git clone "https://${REPO}" "${REPO_NAME}"; then
    	# Checkout the commit, if one was set.
    	if [[ -n "${HASH}" ]]; then
    		pushd ./"${REPO_NAME}"
    		git fetch
    		git checkout "${HASH}"
    		popd
    	fi
        # Change to the repository directory
        # Check if the build script exists and is executable
        if [[ -x "./${REPO_NAME}/scripts/build.sh" ]]; then
          (
            echo "Running build script for ${REPO}..."
            cd "${REPO_NAME}"
            ./scripts/build.sh
            echo "Build script for ${REPO} complete!"
          )
        else
            echo "No build script found in ${REPO}"
        fi

        OBOT_SERVER_VERSIONS="$(cat <<VERSIONS
"${REPO}": "$(cd "${REPO_NAME}" && git rev-parse --short HEAD)"
${OBOT_SERVER_VERSIONS}
VERSIONS
)"

    else
        echo "Failed to clone $REPO. Aborting..."
        exit 1
    fi

    REGISTRY_REMAP+=("${REPO}=/${REPO_DIR}")
    LOCAL_REGISTRIES+=("/${REPO_DIR}")
done

cd ..
for pj in $(find obot-tools -name package.json | grep -v node_modules); do
  if [ $(basename $(dirname $pj)) == common ]; then
    continue
  fi
  (
    cd $(dirname $pj)
    echo Building $PWD
    pnpm i
  )
done
cd obot-tools

if [ ! -e workspace-provider ]; then
    git clone --depth=1 https://github.com/gptscript-ai/workspace-provider
fi
cd workspace-provider
go build -ldflags="-s -w" -o bin/gptscript-go-tool .
REGISTRY_REMAP+=('github.com/gptscript-ai/workspace-provider=/obot-tools/workspace-provider')
OBOT_SERVER_VERSIONS="$(cat <<VERSIONS
"github.com/gptscript-ai/workspace-provider": "$(git rev-parse --short HEAD)"
${OBOT_SERVER_VERSIONS}
VERSIONS
)"
cd ..

if [ ! -e datasets ]; then
    git clone --depth=1 https://github.com/gptscript-ai/datasets
fi
cd datasets
go build -ldflags="-s -w" -o bin/gptscript-go-tool .
REGISTRY_REMAP+=('github.com/gptscript-ai/datasets=/obot-tools/datasets')
OBOT_SERVER_VERSIONS="$(cat <<VERSIONS
"github.com/gptscript-ai/datasets": "$(git rev-parse --short HEAD)"
${OBOT_SERVER_VERSIONS}
VERSIONS
)"
cd ..

if [ ! -e aws-encryption-provider ]; then
    git clone --depth=1 https://github.com/kubernetes-sigs/aws-encryption-provider
fi
cd aws-encryption-provider
go build -o "${BIN_DIR}/aws-encryption-provider" cmd/server/main.go
OBOT_SERVER_VERSIONS="$(cat <<VERSIONS
"github.com/kubernetes-sigs/aws-encryption-provider": "$(git rev-parse --short HEAD)"
${OBOT_SERVER_VERSIONS}
VERSIONS
)"
cd ..

if [ ! -e kubernetes-kms ]; then
    git clone --depth=1 https://github.com/Azure/kubernetes-kms
fi
cd kubernetes-kms
go build -ldflags="-s -w" -o "${BIN_DIR}/azure-encryption-provider" cmd/server/main.go
OBOT_SERVER_VERSIONS="$(cat <<VERSIONS
"github.com/Azure/kubernetes-kms": "$(git rev-parse --short HEAD)"
${OBOT_SERVER_VERSIONS}
VERSIONS
)"
cd ..

if [ ! -e k8s-cloudkms-plugin ]; then
	git clone --depth=1 https://github.com/obot-platform/k8s-cloudkms-plugin
fi
cd k8s-cloudkms-plugin
go build -ldflags "-s -w -extldflags 'static'" -installsuffix cgo -tags netgo -o "${BIN_DIR}/gcp-encryption-provider" cmd/k8s-cloudkms-plugin/main.go
OBOT_SERVER_VERSIONS="$(cat <<VERSIONS
"github.com/obot-platform/k8s-cloudkms-plugin": "$(git rev-parse --short HEAD)"
${OBOT_SERVER_VERSIONS}
VERSIONS
)"
cd ../..

if ! command -v uv; then
    pip install uv
fi

if [ ! -e obot-tools/venv ]; then
    uv venv obot-tools/venv
fi

source obot-tools/venv/bin/activate
find obot-tools -name requirements.txt -exec cat {} \; -exec echo \; | sort -u > requirements.txt
uv pip install -r requirements.txt

cd obot-tools
cat <<EOF > .envrc.tools
export GPTSCRIPT_SYSTEM_TOOLS_DIR=/obot-tools/
export GPTSCRIPT_TOOL_REMAP="$(IFS=','; echo "${REGISTRY_REMAP[*]}")"
export OBOT_SERVER_TOOL_REGISTRIES="${TOOL_REGISTRY_REPOS}"
export OBOT_SERVER_VERSIONS="${OBOT_SERVER_VERSIONS}"
export TOOLS_VENV_BIN=/obot-tools/venv/bin
EOF