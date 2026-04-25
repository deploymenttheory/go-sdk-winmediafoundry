#!/bin/sh
# docker-entrypoint.sh — generates TLS certificates (if absent) and starts
# the winupdate API server.
set -e

CERTS_DIR="${CERTS_DIR:-/certs}"
DB_PATH="${DB_PATH:-/data/winupdate.db}"
ADDR="${ADDR:-:8443}"
NO_WATCHER="${NO_WATCHER:-true}"

# ── Ensure data directory exists ─────────────────────────────────────────────
mkdir -p "$(dirname "$DB_PATH")"

# ── Generate TLS certificates if not already present ─────────────────────────
if [ ! -f "$CERTS_DIR/server.crt" ] || [ ! -f "$CERTS_DIR/ca.crt" ]; then
    echo "==> Generating self-signed CA + server + client certificates in $CERTS_DIR ..."
    mkdir -p "$CERTS_DIR"

    # ── CA ─────────────────────────────────────────────────────────────────────
    cat > /tmp/ca.cnf <<'EOF'
[req]
distinguished_name = dn
x509_extensions    = v3_ca
prompt             = no
[dn]
CN = WinUpdate-CA
[v3_ca]
subjectKeyIdentifier   = hash
authorityKeyIdentifier = keyid:always,issuer
basicConstraints       = critical, CA:true
keyUsage               = critical, digitalSignature, cRLSign, keyCertSign
EOF
    openssl genrsa -out "$CERTS_DIR/ca.key" 4096 2>/dev/null
    openssl req -new -x509 -days 3650 \
        -key "$CERTS_DIR/ca.key" -out "$CERTS_DIR/ca.crt" \
        -config /tmp/ca.cnf

    # ── Server cert (SAN covers localhost + container hostname) ───────────────
    HOSTNAME=$(hostname 2>/dev/null || echo "winupdate")
    cat > /tmp/server.cnf <<EOF
[req]
distinguished_name = dn
req_extensions     = v3_req
prompt             = no
[dn]
CN = localhost
[v3_req]
subjectAltName    = @alt_names
keyUsage          = digitalSignature, keyEncipherment
extendedKeyUsage  = serverAuth
[alt_names]
DNS.1 = localhost
DNS.2 = ${HOSTNAME}
DNS.3 = winupdate
IP.1  = 127.0.0.1
EOF
    openssl genrsa -out "$CERTS_DIR/server.key" 2048 2>/dev/null
    openssl req -new -key "$CERTS_DIR/server.key" -out /tmp/server.csr \
        -config /tmp/server.cnf
    openssl x509 -req -days 365 \
        -in /tmp/server.csr \
        -CA "$CERTS_DIR/ca.crt" -CAkey "$CERTS_DIR/ca.key" -CAcreateserial \
        -out "$CERTS_DIR/server.crt" \
        -extensions v3_req -extfile /tmp/server.cnf

    # ── Client cert ────────────────────────────────────────────────────────────
    cat > /tmp/client.cnf <<'EOF'
[req]
distinguished_name = dn
req_extensions     = v3_req
prompt             = no
[dn]
CN = winupdate-client
[v3_req]
keyUsage         = digitalSignature
extendedKeyUsage = clientAuth
EOF
    openssl genrsa -out "$CERTS_DIR/client.key" 2048 2>/dev/null
    openssl req -new -key "$CERTS_DIR/client.key" -out /tmp/client.csr \
        -config /tmp/client.cnf
    openssl x509 -req -days 365 \
        -in /tmp/client.csr \
        -CA "$CERTS_DIR/ca.crt" -CAkey "$CERTS_DIR/ca.key" -CAcreateserial \
        -out "$CERTS_DIR/client.crt" \
        -extensions v3_req -extfile /tmp/client.cnf

    rm -f /tmp/ca.cnf /tmp/server.cnf /tmp/server.csr \
           /tmp/client.cnf /tmp/client.csr \
           "$CERTS_DIR"/*.srl

    echo "==> Certificates ready:"
    echo "    CA:     $CERTS_DIR/ca.crt"
    echo "    Server: $CERTS_DIR/server.crt  (+ server.key)"
    echo "    Client: $CERTS_DIR/client.crt  (+ client.key)"
fi

# ── Assemble serve command args ───────────────────────────────────────────────
set -- winupdate serve \
    --db     "$DB_PATH" \
    --addr   "$ADDR" \
    --cert   "$CERTS_DIR/server.crt" \
    --key    "$CERTS_DIR/server.key" \
    --ca     "$CERTS_DIR/ca.crt"

if [ "$NO_WATCHER" = "true" ]; then
    set -- "$@" --no-watcher
fi

echo "==> Starting: $*"
exec "$@"
