package database_test

import (
	"testing"

	"github.com/edysupardi/parkirpintar/pkg/database"
	"github.com/stretchr/testify/assert"
)

func TestConfig_Validate_MissingHost(t *testing.T) {
	cfg := database.Config{Name: "db", User: "user", Password: "pass"}
	err := cfg.ExportValidate()
	assert.ErrorContains(t, err, "host")
}

func TestConfig_Validate_MissingName(t *testing.T) {
	cfg := database.Config{Host: "localhost", User: "user", Password: "pass"}
	err := cfg.ExportValidate()
	assert.ErrorContains(t, err, "name")
}

func TestConfig_Validate_MissingUser(t *testing.T) {
	cfg := database.Config{Host: "localhost", Name: "db", Password: "pass"}
	err := cfg.ExportValidate()
	assert.ErrorContains(t, err, "user")
}

func TestConfig_Validate_MissingPassword(t *testing.T) {
	cfg := database.Config{Host: "localhost", Name: "db", User: "user"}
	err := cfg.ExportValidate()
	assert.ErrorContains(t, err, "password")
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := database.Config{Host: "localhost", Name: "db", User: "user", Password: "pass"}
	err := cfg.ExportValidate()
	assert.NoError(t, err)
}

func TestConfig_DSN_ContainsFields(t *testing.T) {
	cfg := database.Config{
		Host:         "localhost",
		Port:         5432,
		Name:         "parkirpintar",
		User:         "postgres",
		Password:     "secret",
		SSLMode:      "disable",
		MaxOpenConns: 25,
		MaxIdleConns: 5,
	}
	dsn := cfg.ExportDSN()
	assert.Contains(t, dsn, "localhost")
	assert.Contains(t, dsn, "parkirpintar")
	assert.Contains(t, dsn, "postgres")
	assert.Contains(t, dsn, "disable")
	assert.Contains(t, dsn, "25")
	assert.Contains(t, dsn, "5")
}
