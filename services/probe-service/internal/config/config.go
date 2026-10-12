package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	sharedConfig "github.com/danielkamin/isp-monitor/pkg/config"
	"github.com/danielkamin/isp-monitor/pkg/secrets"
)

type DBConfig struct {
	User            string
	Pass            string
	Host            string
	Port            string
	Name            string
	MaxConns        int
	MinConns        int
	MaxConnLifetime time.Duration
}
type GrpcConfig struct {
	Port        string
	TargetHosts []string
}
type ProbeConfig struct {
	ProbeIntervalSeconds int
}
type Config struct {
	Database DBConfig
	GRPC     GrpcConfig
	Probe    ProbeConfig
}

func Load(p secrets.Provider) (*Config, error) {
	dbUser, err := sharedConfig.LoadString(p, "DB_USER")
	if err != nil {
		return nil, err
	}
	dbPass, err := sharedConfig.LoadString(p, "DB_PASS")
	if err != nil {
		return nil, err
	}
	dbHost, err := sharedConfig.LoadString(p, "DB_HOST")
	if err != nil {
		return nil, err
	}
	dbPort := sharedConfig.LoadStringOrDefault(p, "DB_PORT", "5432")
	dbName := sharedConfig.LoadStringOrDefault(p, "DB_NAME", "isp-monitor")
	dbMaxConns, err := strconv.Atoi(sharedConfig.LoadStringOrDefault(p, "DB_MAX_CONNS", "25"))
	if err != nil {
		return nil, err
	}
	dbMinConns, err := strconv.Atoi(sharedConfig.LoadStringOrDefault(p, "DB_MIN_CONNS", "5"))
	if err != nil {
		return nil, err
	}
	dbMaxConnLifetimeStr := sharedConfig.LoadStringOrDefault(p, "DB_MAX_LIFETIME", "5m")
	dbMaxConnLifetime, err := time.ParseDuration(dbMaxConnLifetimeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_MAX_CONN_LIFETIME: %w", err)
	}
	dbConfig := DBConfig{
		User:            dbUser,
		Pass:            dbPass,
		Host:            dbHost,
		Port:            dbPort,
		Name:            dbName,
		MaxConns:        dbMaxConns,
		MinConns:        dbMinConns,
		MaxConnLifetime: time.Duration(dbMaxConnLifetime),
	}

	grpcPort := sharedConfig.LoadStringOrDefault(p, "GRPC_PORT", "50051")

	var grpcTargetHosts []string
	if hosts := sharedConfig.LoadStringOrDefault(p, "GRPC_HOSTS", ""); hosts != "" {
		grpcTargetHosts = strings.Split(hosts, ",")
	}

	probeIntervalSecondsConfig, _ := strconv.Atoi(sharedConfig.LoadStringOrDefault(p, "PROBE_INTERVAL_SECONDS", "120"))
	return &Config{
		Database: dbConfig,
		GRPC: GrpcConfig{
			Port:        grpcPort,
			TargetHosts: grpcTargetHosts,
		},
		Probe: ProbeConfig{
			ProbeIntervalSeconds: probeIntervalSecondsConfig,
		},
	}, nil
}
