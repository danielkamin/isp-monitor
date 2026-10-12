SET
    search_path TO probe;

CREATE TYPE device_type AS ENUM ('router', 'dvr', 'alarm', 'generic');

CREATE TYPE probe_protocol AS ENUM ('icmp', 'tcp', 'traceroute', 'mtr', 'http');

CREATE TABLE
    IF NOT EXISTS isp_providers (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        isp_name TEXT,
        isp_asn BIGINT
    );

CREATE TABLE
    IF NOT EXISTS sites (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        name TEXT NOT NULL,
        isp_id UUID NOT NULL REFERENCES isp_providers (id),
        active BOOLEAN NOT NULL DEFAULT true,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW (),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW () --jako trigger albo update w kodzie
    );

CREATE TABLE
    IF NOT EXISTS targets (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        site_id UUID NOT NULL REFERENCES sites (id),
        name TEXT NOT NULL,
        type device_type NOT NULL DEFAULT 'generic',
        ip INET NOT NULL,
        port INT CHECK (port BETWEEN 1 AND 65535),
        protocol probe_protocol NOT NULL DEFAULT 'icmp',
        active BOOLEAN NOT NULL DEFAULT true,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW ()
    );

CREATE TABLE
    IF NOT EXISTS probes (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        target_id UUID NOT NULL REFERENCES targets (id),
        latency_ms INTEGER,
        packet_loss FLOAT NOT NULL DEFAULT 0,
        success BOOLEAN NOT NULL,
        error_msg TEXT,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW ()
    );

CREATE INDEX IF NOT EXISTS idx_sites_isp_name ON sites (isp_name);

CREATE INDEX IF NOT EXISTS idx_sites_active ON sites (active);

CREATE INDEX IF NOT EXISTS idx_targets_site_id ON targets (site_id);

CREATE INDEX IF NOT EXISTS idx_targets_active ON targets (active);

CREATE INDEX IF NOT EXISTS idx_probes_target_id ON probes (target_id);

CREATE INDEX IF NOT EXISTS idx_probes_created_at ON probes (created_at DESC);