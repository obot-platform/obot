#!/bin/bash
set -e -x -o pipefail

BIN_DIR=${BIN_DIR:-./bin}

cd $(dirname $0)/..

if [ ! -e acorn-tools ]; then
    git clone --depth=1 https://github.com/acorn-io/tools acorn-tools
fi

./acorn-tools/scripts/build.sh

for pj in $(find acorn-tools -name package.json | grep -v node_modules); do
    if [ $(basename $(dirname $pj)) == common ]; then
        continue
    fi
    (
        cd $(dirname $pj)
        echo Building $PWD
        pnpm i
    )
done

cd acorn-tools
if [ ! -e workspace-provider ]; then
    git clone --depth=1 https://github.com/gptscript-ai/workspace-provider
fi

cd workspace-provider
go build -ldflags="-s -w" -o bin/gptscript-go-tool .

cd ..

if [ ! -e datasets ]; then
    git clone --depth=1 https://github.com/gptscript-ai/datasets
fi

cd datasets
go build -ldflags="-s -w" -o bin/gptscript-go-tool .

cd ..

if [ ! -e aws-encryption-provider ]; then
    git clone --depth=1 https://github.com/kubernetes-sigs/aws-encryption-provider
fi

cd aws-encryption-provider
go build -o ${BIN_DIR}/aws-encryption-provider cmd/server/main.go

cd ../..

if ! command -v uv; then
    pip install uv
fi

if [ ! -e acorn-tools/venv ]; then
    uv venv acorn-tools/venv
fi

source acorn-tools/venv/bin/activate

find acorn-tools -name requirements.txt -exec cat {} \; -exec echo \; | sort -u > requirements.txt
uv pip install -r requirements.txt
