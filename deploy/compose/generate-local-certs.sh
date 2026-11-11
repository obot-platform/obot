#!/bin/sh
set -eu

directory="${1:-runtime/mTLS}"
mkdir -p "$directory"

openssl req -x509 -newkey rsa:2048 -nodes -days 2 \
  -subj "/CN=OpenAPI local demo CA" -keyout "$directory/ca.key" -out "$directory/ca.crt"

openssl req -newkey rsa:2048 -nodes \
  -subj "/CN=openapi-worker" -keyout "$directory/server.key" -out "$directory/server.csr"
printf "subjectAltName=DNS:openapi-worker,DNS:localhost,IP:127.0.0.1\nbasicConstraints=CA:FALSE\n" > "$directory/server.ext"
openssl x509 -req -in "$directory/server.csr" -CA "$directory/ca.crt" -CAkey "$directory/ca.key" \
  -CAcreateserial -days 2 -extfile "$directory/server.ext" -out "$directory/server.crt"

openssl req -newkey rsa:2048 -nodes \
  -subj "/CN=obot" -keyout "$directory/client.key" -out "$directory/client.csr"
printf "basicConstraints=CA:FALSE\n" > "$directory/client.ext"
openssl x509 -req -in "$directory/client.csr" -CA "$directory/ca.crt" -CAkey "$directory/ca.key" \
  -CAcreateserial -days 2 -extfile "$directory/client.ext" -out "$directory/client.crt"

cp "$directory/ca.crt" "$directory/client-ca.crt"
cp "$directory/ca.crt" "$directory/server-ca.crt"

openssl req -newkey rsa:2048 -nodes \
  -subj "/CN=local-prober" -keyout "$directory/prober.key" -out "$directory/prober.csr"
printf "basicConstraints=CA:FALSE\n" > "$directory/prober.ext"
openssl x509 -req -in "$directory/prober.csr" -CA "$directory/ca.crt" -CAkey "$directory/ca.key" \
  -CAcreateserial -days 2 -extfile "$directory/prober.ext" -out "$directory/prober.crt"

chmod 0444 "$directory"/*.crt
chmod 0444 "$directory"/*.key
