-- probe_user: DML only, admin tworzy tabele przez migracje
CREATE USER probe_user WITH PASSWORD 'dev_password';
GRANT CONNECT ON DATABASE "isp-monitor" TO probe_user;
GRANT USAGE ON SCHEMA probe TO probe_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA probe GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO probe_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA probe GRANT USAGE, SELECT ON SEQUENCES TO probe_user;

-- analytics_user: DML only
CREATE USER analytics_user WITH PASSWORD 'dev_password';
GRANT CONNECT ON DATABASE "isp-monitor" TO analytics_user;
GRANT USAGE ON SCHEMA analytics TO analytics_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA analytics GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO analytics_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA analytics GRANT USAGE, SELECT ON SEQUENCES TO analytics_user;

-- api_user: SELECT only na probe i analytics
CREATE USER api_user WITH PASSWORD 'dev_password';
GRANT CONNECT ON DATABASE "isp-monitor" TO api_user;
GRANT USAGE ON SCHEMA probe, analytics TO api_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA probe GRANT SELECT ON TABLES TO api_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA analytics GRANT SELECT ON TABLES TO api_user;
