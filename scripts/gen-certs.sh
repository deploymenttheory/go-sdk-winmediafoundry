#!/bin/sh
# gen-certs.sh — generates a self-signed CA, server certificate, and client
# certificate for local development / testing.
#
# The resulting files are used by:
#   winupdate serve --cert certs/server.crt --key certs/server.key --ca certs/ca.crt
#   curl --cert certs/client.crt --key certs/client.key --cacert certs/ca.crt ...
#
# Usage:
#   ./scripts/gen-certs.sh            # writes to ./certs/
#   ./scripts/gen-certs.sh /tmp/certs # writes to /tmp/certs/
set -e

DIR="${1:-./certs}"
mkdir -p "$DIR"

echo "==> Generating CA ..."
cat > "$DIR/ca.cnf" <<'EOF'
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
openssl genrsa -out "$DIR/ca.key" 4096 2>/dev/null
openssl req -new -x509 -days 3650 \
    -key "$DIR/ca.key" -out "$DIR/ca.crt" -config "$DIR/ca.cnf"

echo "==> Generating server certificate ..."
cat > "$DIR/server.cnf" <<'EOF'
[req]
distinguished_name = dn
req_extensions     = v3_req
prompt             = no
[dn]
CN = localhost
[v3_req]
subjectAltName   = @alt_names
keyUsage         = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
[alt_names]
DNS.1 = localhost
IP.1  = 127.0.0.1
EOF
openssl genrsa -out "$DIR/server.key" 2048 2>/dev/null
openssl req -new -key "$DIR/server.key" -out "$DIR/server.csr" -config "$DIR/server.cnf"
openssl x509 -req -days 365 \
    -in "$DIR/server.csr" \
    -CA "$DIR/ca.crt" -CAkey "$DIR/ca.key" -CAcreateserial \
    -out "$DIR/server.crt" \
    -extensions v3_req -extfile "$DIR/server.cnf"

echo "==> Generating client certificate ..."
cat > "$DIR/client.cnf" <<'EOF'
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
openssl genrsa -out "$DIR/client.key" 2048 2>/dev/null
openssl req -new -key "$DIR/client.key" -out "$DIR/client.csr" -config "$DIR/client.cnf"
openssl x509 -req -days 365 \
    -in "$DIR/client.csr" \
    -CA "$DIR/ca.crt" -CAkey "$DIR/ca.key" -CAcreateserial \
    -out "$DIR/client.crt" \
    -extensions v3_req -extfile "$DIR/client.cnf"

# Clean up temp files
rm -f "$DIR/ca.cnf"     "$DIR/ca.srl" \
      "$DIR/server.cnf"  "$DIR/server.csr"  "$DIR/server.srl" \
      "$DIR/client.cnf"  "$DIR/client.csr"  "$DIR/client.srl"

echo ""
echo "Certificates written to $DIR/"
echo "  CA cert:     $DIR/ca.crt"
echo "  Server cert: $DIR/server.crt  (private key: server.key)"
echo "  Client cert: $DIR/client.crt  (private key: client.key)"
echo ""
echo "Start the server:"
echo "  winupdate serve --cert $DIR/server.crt --key $DIR/server.key --ca $DIR/ca.crt"
echo ""
echo "Call APIs with mTLS:"
echo "  curl --cert $DIR/client.crt --key $DIR/client.key --cacert $DIR/ca.crt https://localhost:8443/v1/builds"
