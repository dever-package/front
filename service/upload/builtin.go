package upload

import (
	"context"
	"fmt"

	"github.com/shemic/dever/server"

	"github.com/dever-package/front/service/internal/providerpayload"
	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

type ImportURLResourceInput struct {
	RuleID     uint64
	URL        string
	Name       string
	Mime       string
	Kind       string
	BizKey     string
	BizName    string
	CategoryID uint64
}

type UploadBuiltinService struct{}

func (UploadBuiltinService) ProviderImportURLResource(c *server.Context, params []any) any {
	if c == nil {
		panic("资源转存需要当前登录上下文")
	}
	input := importURLResourceInputFromPayload(providerpayload.Map(params))
	if err := requireUploadCreateAccess(c, uploadCreateAccessInput{
		BizKey:     input.BizKey,
		Kind:       input.Kind,
		CategoryID: input.CategoryID,
	}); err != nil {
		panic(err)
	}
	file, err := ImportURLResource(c.Context(), input)
	if err != nil {
		panic(err)
	}
	logUploadFile(c, uint64(providerpayload.Uint64(file, "id")), input)
	return file
}

func ImportURLResource(ctx context.Context, input ImportURLResourceInput) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if input.RuleID == 0 {
		return nil, fmt.Errorf("上传规则不能为空")
	}
	release, err := acquireImportURLSlot()
	if err != nil {
		return nil, err
	}
	defer release()

	fileRecord, err := importURLUploadWithProgress(ctx, uploadImportURLInput{
		RuleID:     input.RuleID,
		URL:        input.URL,
		Name:       input.Name,
		Mime:       input.Mime,
		Kind:       input.Kind,
		BizKey:     input.BizKey,
		BizName:    input.BizName,
		CategoryID: input.CategoryID,
	}, nil)
	if err != nil {
		return nil, err
	}
	payload := uploadrepo.BuildUploadFilePayload(fileRecord)
	if outputKey := uploadOutputKey(input.Kind, payload); outputKey != "" {
		payload[outputKey] = []any{payload["url"]}
	}
	return payload, nil
}

func importURLResourceInputFromPayload(payload map[string]any) ImportURLResourceInput {
	return ImportURLResourceInput{
		RuleID:     providerpayload.Uint64(payload, "rule_id", "ruleId"),
		URL:        providerpayload.Text(payload, "url", "source_url", "sourceUrl"),
		Name:       providerpayload.Text(payload, "name", "filename", "fileName"),
		Mime:       providerpayload.Text(payload, "mime", "mime_type", "mimeType"),
		Kind:       providerpayload.Text(payload, "kind", "type"),
		BizKey:     providerpayload.Text(payload, "biz_key", "bizKey"),
		BizName:    providerpayload.Text(payload, "biz_name", "bizName"),
		CategoryID: providerpayload.Uint64(payload, "category_id", "categoryId"),
	}
}
