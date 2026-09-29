# Storage Layer Troubleshooting Guide

This guide helps you find and correct common storage-layer problems.

## Startup Issues

### "Failed to Initialize Storage"

**Symptoms**: Application exits at startup with storage initialization error

**Examine the configuration:**

1. Make sure that the storage backend is configured.
   ```bash
   # Check config file
   cat config.yaml | grep -A 5 "^storage:"
   # Should show: backend: memory or backend: postgres
   ```

2. If you use PostgreSQL, make sure that the connection URL is set. Then connect with
   `psql`.
   ```bash
   # Verify connection URL is set
   echo $IDENTITY_BROKER_STORAGE_POSTGRES_URL
   # Should not be empty

   # Test PostgreSQL connection
   psql $IDENTITY_BROKER_STORAGE_POSTGRES_URL -c "SELECT 1"
   ```

3. Start the broker with debug logging.
   ```bash
   agentic-identity-broker --log-level=debug --config config.yaml
   ```

### "Database Connection Failed"

**Symptoms**: `ErrorKindConnection` error at startup

**Possible Causes**:

| Cause | Fix |
|-------|-----|
| PostgreSQL server not running | `docker run -d -e POSTGRES_PASSWORD=password postgres:15` |
| Wrong hostname in connection URL | Check DNS resolution: `nslookup db.example.com` |
| Port unreachable | `telnet localhost 5432` |
| SSL/TLS misconfiguration | Try `sslmode=disable` temporarily to diagnose |
| Firewall blocking connection | Check security groups/firewall rules |

**Diagnostic commands:**
```bash
# Test connection with psql directly
psql -h localhost -U postgres -d identity_broker

# Check network connectivity
telnet localhost 5432

# Verify connection URL format
# Expected: postgresql://user:password@host:port/dbname[?params]
```

### "Schema Not Found" or "Table Not Found"

**Symptoms**: `ErrorKindValidation` with message about missing tables

**Solution:** Start database migrations.
```bash
# Create database and schema
agentic-identity-broker migrate up

# Verify schema was created
psql $IDENTITY_BROKER_STORAGE_POSTGRES_URL -c "
  SELECT tablename FROM pg_tables WHERE schemaname='public';"
```

Expected tables:
- `schema_migrations` records applied migrations.
- `users` stores user entities.

## Runtime Issues

### Timeout Errors

**Symptoms**: `ErrorKindTimeout` with "operation exceeded timeout"

**Likely Causes**:

1. **Database is slow**
   - Check PostgreSQL performance: `EXPLAIN ANALYZE SELECT * FROM users;`
   - Monitor CPU/memory: `top` or cloud provider dashboard
   - Check slow query log

2. **Connection pool exhausted**
   ```bash
   # Check active connections
   psql $IDENTITY_BROKER_STORAGE_POSTGRES_URL -c "
     SELECT count(*) as active_connections
     FROM pg_stat_activity;"
   ```

3. **Network latency**
   - Measure latency: `ping -c 5 db.example.com`
   - Check traceroute: `traceroute db.example.com`
   - Move application closer to database

**Solution: Increase timeouts**
```yaml
storage:
  backend: postgres
  timeouts:
    read: 10s    # Increase from 5s
    write: 20s   # Increase from 10s
```

### High memory use with the in-memory backend

**Symptoms:** Memory use increases with the memory backend.

**Cause:** The in-memory adapter does not evict data.

**Solutions:**

1. Restart the application at intervals.
   ```bash
   # Use deployment's rolling restart
   kubectl rollout restart deployment/agentic-identity-broker
   ```

2. Use PostgreSQL in production.
   ```yaml
   # configs/config.prod.yaml
   storage:
     backend: postgres  # Better for persistent, scalable storage
   ```

3. Delete old users in development only.
   ```bash
   curl -X DELETE http://localhost:14000/admin/users/old_user_id
   ```

### Duplicate Key Conflicts

**Symptoms**: `ErrorKindConflict` when creating users

**Root Cause**: User with that ID already exists

**Solution: Determine whether the user exists**
```bash
# Check if user exists
curl http://localhost:8000/users/user123

# If 404, then safe to create
# If 200, user exists - update instead or use different ID
```

## Connection Pool Issues

### "Too Many Connections"

**Symptoms**: PostgreSQL error "too many connections"

**Solution - Reduce Pool Size**:

The default pool configuration is:
- Max open: 25
- Max idle: 5
- Max lifetime: 1 hour
- Max idle time: 15 minutes

To reduce load, use one of these approaches:

1. Decrease the number of Agentic Identity Broker instances.
2. Use a connection pooler such as PgBouncer or pgpool.
   ```yaml
   storage:
     postgres:
       connection_url: postgresql://user@pgbouncer:6432/db
   ```

### Connection timeouts

**Symptoms:** Occasional `ErrorKindTimeout` errors.

**Examine:** Connection-pool health.
```bash
# Monitor connections in real-time
watch -n 1 "psql $IDENTITY_BROKER_STORAGE_POSTGRES_URL -c
  'SELECT count(*) FROM pg_stat_activity;'"
```

**Solutions:**
1. If PostgreSQL has connection limits, reduce `Max open connections`.
2. Use a connection pooler to centralize connection management.
3. If connections close too early, increase the idle timeout.

## Security Issues

### Sensitive Data in Logs

**Symptom**: Connection string appears in log output

**Built-in control:**
- The broker redacts connection strings in logs.
- Do not record raw configuration values.
- Use `--log-level=error` in production to reduce log output.

**Examine redaction:**
```bash
agentic-identity-broker --log-level=debug --config configs/config.prod.yaml 2>&1 | grep -i password
# Should output nothing - password should be redacted as "[REDACTED]"
```

### SSL Certificate Verification Failure

**Symptoms**: `ErrorKindConnection` with SSL/certificate error

**Diagnostic commands:**
```bash
# Test connection with SSL verification
psql "postgresql://user@host/db?sslmode=verify-full" -c "SELECT 1"

# If fails, check certificate
openssl s_client -connect host:5432 -showcerts

# Verify certificate is valid
openssl x509 -in /path/to/cert.pem -text -noout
```

**Solutions:**

1. Accept a self-signed certificate.
   ```yaml
   storage:
     postgres:
       connection_url: "postgresql://...?sslmode=require&sslrootcert=/path/to/ca.pem"
   ```

2. Disable SSL in development only.
   ```yaml
   storage:
     postgres:
       connection_url: "postgresql://...?sslmode=disable"
   ```

3. Correct the certificate.
   - Make sure that the certificate CN matches the hostname.
   - Update the certificate expiration.
   - Install intermediate certificates.

## Data Integrity Issues

### Users Missing After Restart

**With the in-memory backend:**
- This behavior is expected. The adapter loses data on restart.
- Use PostgreSQL for persistent storage.
- You can load seed data at startup.

**With PostgreSQL:**

1. Make sure that data is in the database.
   ```bash
   psql $IDENTITY_BROKER_STORAGE_POSTGRES_URL -c "
     SELECT id, email FROM users LIMIT 5;"
   ```

2. Make sure that the application does not delete data.
   - Examine recent changes.
   - Examine logs for `DELETE` statements.
   - Look for cleanup or purge processes.

3. Make sure that a backup is valid.
   ```bash
   # List backups
   pg_basebackup -D /tmp/backup -v
   ```

### Concurrent access problems

**Symptoms:** Occasional `ErrorKindConflict` or stale data.

**In-memory adapter:**
- Uses `sync.RWMutex` for concurrent access.
- Run `go test -race ./...` to check for races in your build and configuration.

**PostgreSQL adapter:**
- The database controls concurrent access through transactions.
- If the problem continues, examine locks:
  ```bash
  psql -c "SELECT * FROM pg_stat_activity;" # Check for locks
  pg_locks    # Check for blocking queries
  ```

## Performance Tuning

**Examine query performance:**
```bash
psql -c "
EXPLAIN ANALYZE
SELECT id, email, created_at, updated_at FROM users LIMIT 100;"
```

**Solutions:**
1. Create indexes on frequently searched columns.
2. If the database returns data, increase the `read` timeout.
3. Move the database closer to the application to reduce network latency.

### Slow write operations

**Examine write performance:**
```bash
time psql -c "INSERT INTO users (id, email, created_at, updated_at)
              VALUES ('test', 'test@example.com', NOW(), NOW());"
```

**Solutions:**
1. Examine disk I/O with `iostat` or cloud metrics.
2. Increase the `write` timeout temporarily.
3. If transactions are available, batch writes in a transaction.
4. Archive old data when the table is large.

## Debugging with Environment Variables

Enable detailed logging:
```bash
export IDENTITY_BROKER_LOG_LEVEL=debug
export IDENTITY_BROKER_LOG_FORMAT=json

agentic-identity-broker --config config.yaml
```

JSON logs can be parsed and analyzed:
```bash
# Filter for storage errors
agentic-identity-broker | jq 'select(.component=="storage")'

# Count errors by type
agentic-identity-broker | jq -r '.error_kind' | sort | uniq -c
```

## Testing Connectivity

### Script to examine storage setup

```bash
#!/bin/bash

echo "Testing storage connectivity..."

# Check in-memory backend
echo "Testing in-memory backend..."
cat > test.yaml <<EOF
storage:
  backend: memory
  timeouts:
    read: 5s
    write: 10s
EOF

agentic-identity-broker --config test.yaml &
sleep 2
curl -f http://localhost:8000/health || echo "Health check failed"
pkill -f agentic-identity-broker

# Check PostgreSQL backend
if [ ! -z "$IDENTITY_BROKER_STORAGE_POSTGRES_URL" ]; then
  echo "Testing PostgreSQL backend..."
  cat > test.yaml <<EOF
storage:
  backend: postgres
  postgres:
    connection_url: $IDENTITY_BROKER_STORAGE_POSTGRES_URL
  timeouts:
    read: 5s
    write: 10s
EOF

  agentic-identity-broker --config test.yaml &
  sleep 3
  curl -f http://localhost:8000/health || echo "PostgreSQL health check failed"
  pkill -f agentic-identity-broker
fi

echo "Storage connectivity tests complete"
```

## Getting Help

If you cannot correct the problem:

1. Read logs with timestamps.
   ```bash
   agentic-identity-broker --log-level=debug 2>&1 | tee app.log
   ```

2. Record the error context.
   - Record the exact error message and error kind.
   - Record timestamps and operations that failed.
   - Save redacted configuration values.

3. Report the problem with:
   - The error kind and message.
   - The redacted configuration.
   - Debug logs.
   - Steps to reproduce.
   - Environment information: OS, Go version, and PostgreSQL version.

## References

- Storage Architecture: `adrs/004-storage-layer-architecture.md`
- Security Checklist: `SECURITY.md`
- [Extension Guide](./STORAGE_EXTENSION_GUIDE.md)
- PostgreSQL Documentation: https://www.postgresql.org/docs/
