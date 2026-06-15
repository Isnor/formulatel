# CLAUDE.md

[Agent Rules](./AGENTS.md)

## Database Backup Strategy

### Retention and Backup Options

#### 1. TimescaleDB Automatic Retention (Simplest)
```sql
-- Set chunk retention policy to keep data for 7 days; e.g.:
SELECT add_retention_policy('vehicle_data', INTERVAL 7 days);
SELECT add_retention_policy('motion_data', INTERVAL 7 days);
```

#### 2. pg_dump Daily + S3
Use PostgreSQL's built-in backup tool for full backups.

#### 3. TimescaleDB Timeshift + S3 (Recommended)
Timescale provides `timeshift` for time-travel backups.

```yaml
# k8s CronJob for automated S3 backups
apiVersion: batch/v1
kind: CronJob
metadata:
  name: timescale-backup
  namespace: formulatel
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: timescale/timescaledb-ha:pg18
            command: ["/bin/sh", "-c"]
            args:
            - |
              timeshift -n -t daily -f /backup \
                --timescaledb-user=postgres \
                --pguser=postgres --pgpassword=postgres \
                --dbname=postgres \
                --timescaledb-datasource=timescaledb:5432
            env:
            - name: AWS_ACCESS_KEY_ID
              valueFrom:
                secretKeyRef:
                  name: aws-creds
                  key: access-key-id
            - name: AWS_SECRET_ACCESS_KEY
              valueFrom:
                secretKeyRef:
                  name: aws-creds
                  key: secret-access-key
          restartPolicy: Never
```

#### 4. WAL Archiving (Point-in-Time Recovery)
Enable WAL archiving for point-in-time recovery:
```sql
ALTER SYSTEM SET wal_level = 'replica';
ALTER SYSTEM SET archive_mode = on;
ALTER SYSTEM SET archive_command = 'pg_archiveWal %p /tmp/wal/%f';
```

### Recommended Hybrid Approach
1. Keep recent data hot (last 30 days) in main hypertables
2. Timeshift backups to S3 every 6 hours
3. WAL archiving to S3 for point-in-time recovery
4. Retention policy dropping chunks older than 90 days