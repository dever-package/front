package upload

import (
	"strconv"
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	uploadrepo "my/package/front/service/upload/repository"
)

func ListUploadRules(c *server.Context) error {
	ids := parseUploadRuleIDs(c.Input("ids"), c.Input("id"))
	if len(ids) == 0 {
		return c.JSON([]map[string]any{})
	}

	items := make([]map[string]any, 0, len(ids))
	for _, ruleID := range ids {
		rule, err := uploadrepo.FindUploadRule(c.Context(), ruleID)
		if err != nil {
			return c.Error(err)
		}
		items = append(items, map[string]any{
			"id":        rule.ID,
			"name":      rule.Name,
			"accept":    rule.Accept,
			"transport": rule.Transport,
		})
	}

	return c.JSON(items)
}

func parseUploadRuleIDs(values ...string) []uint64 {
	result := make([]uint64, 0, len(values)*2)
	for _, current := range values {
		for _, token := range strings.FieldsFunc(current, func(r rune) bool {
			return r == ',' || r == ';'
		}) {
			id, err := strconv.ParseUint(strings.TrimSpace(token), 10, 64)
			if err != nil || id == 0 {
				continue
			}
			result = append(result, id)
		}
	}
	return util.UniqueUint64s(result)
}
