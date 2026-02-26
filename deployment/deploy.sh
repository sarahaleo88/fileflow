#!/bin/bash
#============================================================
#  FileFlow One-Click Deploy Script
#  https://github.com/sarahaleo88/fileflow
#
#  Supported OS: Debian 12/13, Ubuntu 22.04/24.04
#  Stack:        Go + SQLite + Docker Compose + Nginx + Certbot
#  Architecture: Docker(Go app :8080) → Nginx reverse proxy → HTTPS
#
#  Usage:
#    sudo bash deploy-fileflow.sh
#
#  The script will interactively prompt for:
#    - Domain name
#    - Email (for Let's Encrypt)
#    - Shared secret (for authentication)
#
#  Known issues resolved in this script:
#    1. APP_SECRET_HASH must be Argon2id format (not plain hex)
#    2. Dollar signs ($) in hash must be escaped as ($$) for
#       docker-compose .env files
#    3. software-properties-common may not exist on Debian 13
#    4. argon2 CLI used instead of Python (low memory footprint)
#    5. Old device data must be cleared when changing secrets
#============================================================

set -euo pipefail

# ==================== 默认配置 ====================
REPO_URL="https://github.com/sarahaleo88/fileflow.git"
DEPLOY_DIR="/opt/fileflow"
RATE_LIMIT_RPS=5
SECURE_COOKIES=true
SESSION_TTL_HOURS=12
MAX_WS_MSG_BYTES=262144
# =================================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log()  { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
err()  { echo -e "${RED}[✗]${NC} $1"; exit 1; }
step() { echo -e "\n${CYAN}══════════════════════════════════════${NC}"; echo -e "${CYAN}  $1${NC}"; echo -e "${CYAN}══════════════════════════════════════${NC}"; }

# 生成随机 token
gen_token() { openssl rand -hex 32 2>/dev/null || head -c 64 /dev/urandom | base64 | tr -dc 'a-zA-Z0-9' | head -c 64; }

# 检查 root 权限
[[ $EUID -ne 0 ]] && err "Please run as root: sudo bash deploy-fileflow.sh"

echo -e "${CYAN}"
echo '  _____ _ _      _____ _                '
echo ' |  ___(_) | ___|  ___| | _____      __ '
echo ' | |_  | | |/ _ \ |_  | |/ _ \ \ /\ / / '
echo ' |  _| | | |  __/  _| | | (_) \ V  V /  '
echo ' |_|   |_|_|\___|_|   |_|\___/ \_/\_/   '
echo -e "${NC}"
echo "  One-Click Deploy Script"
echo ""

#--------------------------------------------------
# Interactive configuration
#--------------------------------------------------
step "Configuration"

read -p "Enter your domain (e.g. fileflow.example.com): " DOMAIN
[[ -z "$DOMAIN" ]] && err "Domain cannot be empty"

read -p "Enter your email (for Let's Encrypt SSL): " EMAIL
[[ -z "$EMAIL" ]] && err "Email cannot be empty"

read -p "Enter shared secret for authentication (min 4 chars): " APP_SECRET
[[ ${#APP_SECRET} -lt 4 ]] && err "Secret must be at least 4 characters"

echo ""
echo -e "  Domain:  ${CYAN}${DOMAIN}${NC}"
echo -e "  Email:   ${CYAN}${EMAIL}${NC}"
echo -e "  Secret:  ${CYAN}${APP_SECRET}${NC}"
echo ""
read -p "Confirm and start deployment? [y/N] " CONFIRM
[[ "$CONFIRM" != "y" && "$CONFIRM" != "Y" ]] && err "Deployment cancelled"

echo -e "\n  Deploy to: ${CYAN}https://${DOMAIN}${NC}\n"

#--------------------------------------------------
# Step 1: System update & base tools
#--------------------------------------------------
step "Step 1/8: System update & install base tools"
apt-get update -y && apt-get upgrade -y
apt-get install -y curl wget git ufw \
    apt-transport-https ca-certificates gnupg lsb-release openssl argon2
log "System updated"

#--------------------------------------------------
# Step 2: Install Docker & Docker Compose
#--------------------------------------------------
step "Step 2/8: Install Docker"
if command -v docker &>/dev/null; then
    warn "Docker already installed, skipping"
else
    install -m 0755 -d /etc/apt/keyrings

    # Auto-detect Debian/Ubuntu
    . /etc/os-release
    if [[ "$ID" == "ubuntu" ]]; then
        DOCKER_REPO="https://download.docker.com/linux/ubuntu"
    else
        DOCKER_REPO="https://download.docker.com/linux/debian"
    fi

    curl -fsSL "${DOCKER_REPO}/gpg" | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    chmod a+r /etc/apt/keyrings/docker.gpg

    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] $DOCKER_REPO $VERSION_CODENAME stable" \
        > /etc/apt/sources.list.d/docker.list

    apt-get update -y
    apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    systemctl enable docker && systemctl start docker
    log "Docker installed"
fi

docker --version
docker compose version

#--------------------------------------------------
# Step 3: Clone project
#--------------------------------------------------
step "Step 3/8: Clone project"
if [ -d "$DEPLOY_DIR/.git" ]; then
    warn "Project directory exists, running git pull..."
    cd "$DEPLOY_DIR" && git pull
else
    rm -rf "$DEPLOY_DIR"
    git clone "$REPO_URL" "$DEPLOY_DIR"
fi
cd "$DEPLOY_DIR"
log "Code cloned to: $DEPLOY_DIR"

#--------------------------------------------------
# Step 4: Generate APP_SECRET_HASH (Argon2id)
#--------------------------------------------------
step "Step 4/8: Generate APP_SECRET_HASH (Argon2id)"

# Use argon2 CLI (lightweight, no Python needed, ~300KB)
SALT=$(openssl rand -base64 16)
APP_SECRET_HASH_RAW=$(echo -n "${APP_SECRET}" | argon2 "${SALT}" -id -e)

if [[ "$APP_SECRET_HASH_RAW" == *'$argon2id$'* ]]; then
    log "Argon2id hash generated"
else
    err "Hash generation failed. Check: apt-get install argon2"
fi

# IMPORTANT: Escape $ as $$ to prevent docker-compose from
# interpreting them as variable references in .env files.
# Without this, docker-compose silently corrupts the hash value.
APP_SECRET_HASH=$(echo "$APP_SECRET_HASH_RAW" | sed 's/\$/\$\$/g')

#--------------------------------------------------
# Step 5: Configure environment variables
#--------------------------------------------------
step "Step 5/8: Configure environment variables"

ENV_FILE="$DEPLOY_DIR/deployment/.env"

# Auto-generate tokens
BOOTSTRAP_TOKEN=$(gen_token)
SESSION_KEY=$(gen_token)

# Backup existing .env if present
if [ -f "$ENV_FILE" ]; then
    warn ".env exists, backed up to .env.bak"
    cp "$ENV_FILE" "${ENV_FILE}.bak"
fi

cat > "$ENV_FILE" <<ENVFILE
# === FileFlow Production Config ===
# Generated: $(date '+%Y-%m-%d %H:%M:%S')

# Required
APP_DOMAIN=${DOMAIN}
APP_SECRET_HASH=${APP_SECRET_HASH}
BOOTSTRAP_TOKEN=${BOOTSTRAP_TOKEN}
SESSION_KEY=${SESSION_KEY}

# Optional
RATE_LIMIT_RPS=${RATE_LIMIT_RPS}
SECURE_COOKIES=${SECURE_COOKIES}
SESSION_TTL_HOURS=${SESSION_TTL_HOURS}
MAX_WS_MSG_BYTES=${MAX_WS_MSG_BYTES}
TRUSTED_PROXY_CIDRS=127.0.0.1/32,172.16.0.0/12
ACME_EMAIL=${EMAIL}
ENVFILE

log ".env configured"
echo ""
echo -e "  ${YELLOW}Important - save these credentials:${NC}"
echo -e "  APP_SECRET:      ${CYAN}${APP_SECRET}${NC}"
echo -e "  BOOTSTRAP_TOKEN: ${CYAN}${BOOTSTRAP_TOKEN}${NC}"
echo -e "  SESSION_KEY:     ${CYAN}${SESSION_KEY:0:16}...${NC}"
echo ""

#--------------------------------------------------
# Step 6: Docker build & start
#--------------------------------------------------
step "Step 6/8: Docker build & start"
cd "$DEPLOY_DIR/deployment"

# Uses the project's docker-compose.nginx.yml (Nginx mode)
# This only starts the Go app container, binding to 127.0.0.1:8080
docker compose -f docker-compose.nginx.yml down 2>/dev/null || true
docker compose -f docker-compose.nginx.yml build --no-cache
docker compose -f docker-compose.nginx.yml up -d
log "Docker container started"

# Wait for health check
echo -n "Waiting for service to start"
HEALTHY=false
for i in $(seq 1 30); do
    if curl -sf http://127.0.0.1:8080/healthz > /dev/null 2>&1; then
        echo ""
        log "Health check passed (/healthz)"
        HEALTHY=true
        break
    fi
    echo -n "."
    sleep 3
done
echo ""

if [ "$HEALTHY" = false ]; then
    warn "Service not ready within 90s, showing logs:"
    docker compose -f docker-compose.nginx.yml logs --tail=20
    echo ""
    warn "Check the logs above. Common issues:"
    warn "  - APP_SECRET_HASH format incorrect"
    warn "  - Missing required environment variables"
fi

docker compose -f docker-compose.nginx.yml ps

#--------------------------------------------------
# Step 7: Configure Nginx + Certbot
#--------------------------------------------------
step "Step 7/8: Configure Nginx reverse proxy + HTTPS"

apt-get install -y nginx certbot python3-certbot-nginx
systemctl enable nginx

# Generate Nginx config with the user's domain
# WebSocket path (/ws) gets special treatment with long timeouts
cat > /etc/nginx/sites-available/fileflow <<NGINX
server {
    listen 80;
    listen [::]:80;
    server_name ${DOMAIN};

    # WebSocket support (long-lived connections)
    location /ws {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 86400;
        proxy_send_timeout 86400;
    }

    # API & static assets
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 60;

        # Security headers
        add_header X-Content-Type-Options "nosniff" always;
        add_header X-Frame-Options "DENY" always;
        add_header X-XSS-Protection "1; mode=block" always;
        add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    }
}
NGINX

ln -sf /etc/nginx/sites-available/fileflow /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default 2>/dev/null || true

nginx -t && systemctl reload nginx
log "Nginx configured"

# Request SSL certificate
echo ""
warn "Requesting Let's Encrypt SSL certificate..."
warn "Make sure DNS A record for ${DOMAIN} points to this server's IP"
echo ""
certbot --nginx -d "$DOMAIN" --email "$EMAIL" --agree-tos --non-interactive --redirect \
    && log "SSL certificate obtained!" \
    || warn "SSL failed. After DNS propagation, run: certbot --nginx -d ${DOMAIN}"

systemctl enable certbot.timer 2>/dev/null || true

# Enable HSTS if SSL was configured
if grep -q "443 ssl" /etc/nginx/sites-available/fileflow 2>/dev/null; then
    if ! grep -q "Strict-Transport-Security" /etc/nginx/sites-available/fileflow; then
        sed -i '/ssl_certificate_key/a\    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;' \
            /etc/nginx/sites-available/fileflow
        nginx -t && systemctl reload nginx
        log "HSTS enabled"
    fi
fi

#--------------------------------------------------
# Step 8: Configure firewall
#--------------------------------------------------
step "Step 8/8: Configure firewall (UFW)"
ufw allow 22/tcp    # SSH
ufw allow 80/tcp    # HTTP
ufw allow 443/tcp   # HTTPS
ufw --force enable
log "Firewall configured"

#--------------------------------------------------
# Final verification
#--------------------------------------------------
step "Final Verification"
sleep 2

if curl -sf http://127.0.0.1:8080/healthz > /dev/null 2>&1; then
    log "Local health check: PASS"
else
    warn "Local health check: FAIL (container may still be starting)"
fi

if curl -sf "https://${DOMAIN}/healthz" > /dev/null 2>&1; then
    log "HTTPS check: PASS"
else
    warn "HTTPS check: FAIL (DNS may not have propagated yet)"
fi

#--------------------------------------------------
# Done!
#--------------------------------------------------
echo ""
echo -e "${GREEN}══════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}  ✅ FileFlow deployed successfully!${NC}"
echo -e "${GREEN}══════════════════════════════════════════════════════${NC}"
echo ""
echo -e "  🌐 URL:            https://${DOMAIN}"
echo -e "  📂 Project:        ${DEPLOY_DIR}"
echo -e "  🐳 Docker config:  ${DEPLOY_DIR}/deployment/"
echo -e "  🔐 Env file:       ${DEPLOY_DIR}/deployment/.env"
echo -e "  📊 Health check:   https://${DOMAIN}/healthz"
echo ""
echo -e "${YELLOW}  ═══ Credentials (SAVE THESE!) ═══${NC}"
echo -e "  Shared Secret:     ${APP_SECRET}"
echo -e "  BOOTSTRAP_TOKEN:   ${BOOTSTRAP_TOKEN}"
echo ""
echo -e "${YELLOW}  ═══ Management Commands ═══${NC}"
DC="cd ${DEPLOY_DIR}/deployment && docker compose -f docker-compose.nginx.yml"
echo -e "  Status:    ${DC} ps"
echo -e "  Logs:      ${DC} logs -f"
echo -e "  Restart:   ${DC} restart"
echo -e "  Rebuild:   ${DC} up -d --build"
echo -e "  Stop:      ${DC} down"
echo ""
echo -e "  ${CYAN}Update & redeploy:${NC}"
echo -e "  cd ${DEPLOY_DIR} && git pull && cd deployment && docker compose -f docker-compose.nginx.yml up -d --build"
echo ""
echo -e "  ${CYAN}Edit config:${NC}  nano ${DEPLOY_DIR}/deployment/.env"
echo -e "  ${CYAN}Renew SSL:${NC}    certbot renew --dry-run"
echo ""
