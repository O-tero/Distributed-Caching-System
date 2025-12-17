# 1. Setup environment
cp .env.example .env

# 2. Start everything
./scripts/run_local.sh

# 3. Seed test data
./scripts/seed_data.sh --count 500

# 4. Run load test
./scripts/load_test.sh --mode vegeta --rate 300 --duration 60s

# 5. Backup database
./scripts/backup_db.sh

# 6. Stop everything
./scripts/run_local.sh --


<!-- -->

# 1. Copy environment file
cp .env.example .env

# 2. Generate secure credentials
openssl rand -hex 32  # For API tokens
openssl rand -base64 32  # For passwords

# 3. Edit configuration
nano .env

# 4. Start infrastructure
cd infra/local && docker compose up -d

# 5. Verify
curl http://localhost:9400/health