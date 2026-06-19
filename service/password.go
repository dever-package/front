package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/shemic/dever/orm"
	"github.com/shemic/dever/util"
	"golang.org/x/crypto/bcrypt"

	frontmeta "github.com/dever-package/front/service/meta"
	frontrecord "github.com/dever-package/front/service/record"
)

func hashLegacyMD5Password(password string) string {
	sum := md5.Sum([]byte(strings.TrimSpace(password)))
	return hex.EncodeToString(sum[:])
}

func hashModelPassword(password string) (string, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		return "", fmt.Errorf("密码不能为空")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("密码哈希失败: %w", err)
	}
	return string(hashed), nil
}

func HashPassword(password string) (string, error) {
	return hashModelPassword(password)
}

func VerifyPassword(stored string, password string) bool {
	return verifyStoredPassword(stored, password)
}

func PasswordNeedsUpgrade(stored string) bool {
	return shouldUpgradeStoredPassword(stored)
}

func NormalizeModelPasswordFields(modelName string, data map[string]any, columnLookup map[string]string) error {
	for _, passwordColumn := range resolveModelPasswordColumns(modelName, columnLookup) {
		if err := normalizeModelPasswordColumn(data, passwordColumn); err != nil {
			return err
		}
	}
	return nil
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

func normalizeModelPasswordColumn(data map[string]any, passwordColumn string) error {
	rawValue, exists := data[passwordColumn]
	if !exists {
		return nil
	}

	password := util.ToStringTrimmed(rawValue)
	if password == "" {
		delete(data, passwordColumn)
		return nil
	}

	hashed, err := hashModelPassword(password)
	if err != nil {
		return err
	}
	data[passwordColumn] = hashed
	return nil
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

	if verifyStoredPassword(stored, password) {
		if shouldUpgradeStoredPassword(stored) {
			upgradeAccountPassword(ctx, accountModel, util.ToUint64(accountRow["id"]), password)
		}
		return true
	}

	return false
}

func verifyStoredPassword(stored string, password string) bool {
	stored = util.ToStringTrimmed(stored)
	password = strings.TrimSpace(password)
	if stored == "" || password == "" {
		return false
	}
	if isBcryptPasswordHash(stored) {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)) == nil
	}
	return stored == hashLegacyMD5Password(password)
}

func shouldUpgradeStoredPassword(stored string) bool {
	stored = util.ToStringTrimmed(stored)
	return stored != "" && !isBcryptPasswordHash(stored)
}

func isBcryptPasswordHash(stored string) bool {
	stored = util.ToStringTrimmed(stored)
	if !strings.HasPrefix(stored, "$2a$") && !strings.HasPrefix(stored, "$2b$") && !strings.HasPrefix(stored, "$2y$") {
		return false
	}
	_, err := bcrypt.Cost([]byte(stored))
	return err == nil
}

func upgradeAccountPassword(ctx context.Context, accountModel frontrecord.Model, accountID uint64, password string) {
	if accountID > 0 {
		hashed, err := hashModelPassword(password)
		if err != nil {
			return
		}
		accountModel.Update(ctx, map[string]any{"id": accountID}, map[string]any{
			"password": hashed,
		})
	}
}
