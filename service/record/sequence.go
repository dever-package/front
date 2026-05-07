package record

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shemic/dever/orm"
)

func SyncModelPrimarySequence(ctx context.Context, modelName string) error {
	tableName, primaryColumn, err := loadModelPrimarySequenceInfo(modelName)
	if err != nil {
		return err
	}
	if tableName == "" || primaryColumn == "" {
		return nil
	}

	db, err := orm.Get()
	if err != nil {
		return err
	}
	if normalizeDatabaseDriver(db.DriverName()) != "postgres" {
		return nil
	}

	var sequenceName sql.NullString
	if err := db.QueryRowContext(
		ctx,
		"SELECT pg_get_serial_sequence($1, $2)",
		tableName,
		primaryColumn,
	).Scan(&sequenceName); err != nil {
		return err
	}
	if !sequenceName.Valid || strings.TrimSpace(sequenceName.String) == "" {
		return nil
	}

	statement := fmt.Sprintf(
		"SELECT setval($1::regclass, COALESCE((SELECT MAX(%s) FROM %s), 0) + 1, false)",
		quotePostgresIdentifier(primaryColumn),
		quotePostgresTableName(tableName),
	)
	_, err = db.ExecContext(ctx, statement, sequenceName.String)
	return err
}

func loadModelPrimarySequenceInfo(modelName string) (string, string, error) {
	resourceName := ResourceName(modelName)
	if resourceName == "" {
		return "", "", nil
	}

	entries, err := os.ReadDir(filepath.Join("data", "table"))
	if err != nil {
		return "", "", err
	}

	type schemaColumn struct {
		Name          string `json:"name"`
		Primary       bool   `json:"primary"`
		AutoIncrement bool   `json:"autoIncrement"`
	}
	type tableSchemaFile struct {
		Table   string         `json:"table"`
		Columns []schemaColumn `json:"columns"`
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if name != resourceName+".json" && !strings.HasSuffix(name, "_"+resourceName+".json") {
			continue
		}

		content, readErr := os.ReadFile(filepath.Join("data", "table", name))
		if readErr != nil {
			continue
		}

		var schema tableSchemaFile
		if jsonErr := json.Unmarshal(content, &schema); jsonErr != nil {
			continue
		}

		primaryColumn := ""
		for _, column := range schema.Columns {
			if column.Primary && column.AutoIncrement {
				primaryColumn = strings.TrimSpace(column.Name)
				break
			}
		}
		if primaryColumn == "" {
			for _, column := range schema.Columns {
				if column.Primary {
					primaryColumn = strings.TrimSpace(column.Name)
					break
				}
			}
		}

		return strings.TrimSpace(schema.Table), primaryColumn, nil
	}

	return "", "", nil
}

func normalizeDatabaseDriver(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "postgres", "postgresql", "pgx":
		return "postgres"
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

func quotePostgresIdentifier(name string) string {
	parts := strings.Split(strings.TrimSpace(name), ".")
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(part, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, ".")
}

func quotePostgresTableName(name string) string {
	return quotePostgresIdentifier(name)
}
