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
./scripts/run_local.sh --stop