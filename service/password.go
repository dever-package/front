package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"strings"

	"github.com/shemic/dever/orm"
	"github.com/shemic/dever/util"

	frontmeta "my/package/front/service/meta"
	frontrecord "my/package/front/service/record"
)

func hashPlainPassword(password string) string {
	sum := md5.Sum([]byte(strings.TrimSpace(password)))
	return hex.EncodeToString(sum[:])
}

func NormalizeModelPasswordFields(modelName string, data map[string]any, columnLookup map[string]string) {
	for _, passwordColumn := range resolveModelPasswordColumns(modelName, columnLookup) {
		normalizeModelPasswordColumn(data, passwordColumn)
	}
}

func resolveModelPasswordColumns(modelName string, columnLookup map[string]string) []string {
	fields := frontmeta.ResolveModelFieldsByType(modelName, orm.FieldTypePassword)

	columns := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		column := frontrecord.ResolveColumnName(columnLookup, field)
		if column == "" {
			continue
		}
		if _, exists := seen[column]; exists {
			continue
		}
		seen[column] = struct{}{}
		columns = append(columns, column)
	}
	return columns
}

func normalizeModelPasswordColumn(data map[string]any, passwordColumn string) {
	rawValue, exists := data[passwordColumn]
	if !exists {
		return
	}

	password := util.ToStringTrimmed(rawValue)
	if password == "" {
		delete(data, passwordColumn)
		return
	}

	data[passwordColumn] = hashPlainPassword(password)
}

func verifyAccountPassword(
	ctx context.Context,
	accountModel frontrecord.Model,
	accountRow map[string]any,
	password string,
) bool {
	password = strings.TrimSpace(password)
	if password == "" || len(accountRow) == 0 {
		return false
	}

	stored := util.ToStringTrimmed(accountRow["password"])
	if stored == "" {
		return false
	}

	hashed := hashPlainPassword(password)
	if stored == hashed {
		return true
	}

	if stored != password {
		return false
	}

	accountID := util.ToUint64(accountRow["id"])
	if accountID > 0 {
		accountModel.Update(ctx, map[string]any{"id": accountID}, map[string]any{
			"password": hashed,
		})
	}
	return true
}
