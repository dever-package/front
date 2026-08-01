package upload

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shemic/dever/util"

	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

const (
	builtinTextAcceptName = "文本文件"
	builtinTextAccept     = ".txt,.md,.markdown,.mdown,.mkd,text/plain,text/markdown"
	builtinUserFileRuleID = uint64(7)
)

// EnsureBuiltinTextUploadType keeps existing installations in sync with the
// seed data used by newly created databases.
func EnsureBuiltinTextUploadType(ctx context.Context) error {
	acceptTypeID, err := ensureBuiltinTextAcceptType(ctx)
	if err != nil {
		return err
	}
	return ensureRuleAcceptType(ctx, builtinUserFileRuleID, acceptTypeID)
}

func ensureBuiltinTextAcceptType(ctx context.Context) (uint64, error) {
	model, err := uploadrepo.ResolveAcceptTypeModel()
	if err != nil {
		return 0, err
	}

	row := model.FindMap(ctx, map[string]any{"name": builtinTextAcceptName})
	if len(row) == 0 {
		id := util.ToUint64(model.Insert(ctx, map[string]any{
			"name":       builtinTextAcceptName,
			"accept":     builtinTextAccept,
			"created_at": time.Now(),
		}))
		if id == 0 {
			return 0, fmt.Errorf("创建文本文件允许类型失败")
		}
		return id, nil
	}

	id := util.ToUint64(row["id"])
	if id == 0 {
		return 0, fmt.Errorf("文本文件允许类型无效")
	}
	accept := mergeRequiredAccept(util.ToStringTrimmed(row["accept"]), builtinTextAccept)
	if accept != util.ToStringTrimmed(row["accept"]) {
		model.Update(ctx, map[string]any{"id": id}, map[string]any{"accept": accept})
	}
	return id, nil
}

func ensureRuleAcceptType(ctx context.Context, ruleID, acceptTypeID uint64) error {
	ruleModel, err := uploadrepo.ResolveRuleModel()
	if err != nil {
		return err
	}
	if len(ruleModel.FindMap(ctx, map[string]any{"id": ruleID})) == 0 {
		return fmt.Errorf("用户文件上传规则不存在")
	}

	relationModel, err := uploadrepo.ResolveRuleAcceptTypeModel()
	if err != nil {
		return err
	}
	filters := map[string]any{
		"upload_rule_id": ruleID,
		"accept_type_id": acceptTypeID,
	}
	if len(relationModel.FindMap(ctx, filters)) > 0 {
		return nil
	}
	if relationModel.Insert(ctx, map[string]any{
		"upload_rule_id": ruleID,
		"accept_type_id": acceptTypeID,
		"created_at":     time.Now(),
	}) == 0 {
		return fmt.Errorf("关联文本文件上传规则失败")
	}
	return nil
}

func mergeRequiredAccept(current, required string) string {
	result := uploadrepo.SplitAccept(current)
	seen := make(map[string]struct{}, len(result))
	for _, token := range result {
		seen[token] = struct{}{}
	}
	for _, token := range uploadrepo.SplitAccept(required) {
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		result = append(result, token)
	}
	return strings.Join(result, ",")
}
