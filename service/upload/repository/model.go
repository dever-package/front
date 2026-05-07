package repository

import (
	"fmt"

	frontrecord "my/package/front/service/record"
)

func resolveModel(modelName, label string) (frontrecord.Model, error) {
	model := frontrecord.Resolve(modelName)
	if model == nil {
		return nil, fmt.Errorf("%s模型未注册", label)
	}
	return model, nil
}

func ResolveFileModel() (frontrecord.Model, error) {
	return resolveModel("front.NewUploadFileModel", "上传文件")
}

func ResolveSessionModel() (frontrecord.Model, error) {
	return resolveModel("front.NewUploadSessionModel", "上传会话")
}

func ResolveRuleModel() (frontrecord.Model, error) {
	return resolveModel("front.NewUploadRuleModel", "上传规则")
}

func ResolveStorageModel() (frontrecord.Model, error) {
	return resolveModel("front.NewUploadStorageModel", "上传存储方式")
}

func ResolveBizModel() (frontrecord.Model, error) {
	return resolveModel("front.NewUploadFileBizModel", "资源来源")
}

func ResolveCateModel() (frontrecord.Model, error) {
	return resolveModel("front.NewUploadFileCateModel", "资源分类")
}

func ResolveAcceptTypeModel() (frontrecord.Model, error) {
	return resolveModel("front.NewUploadAcceptTypeModel", "上传允许类型")
}

func ResolveRuleAcceptTypeModel() (frontrecord.Model, error) {
	return resolveModel("front.NewUploadRuleAcceptTypeModel", "上传规则允许类型关联")
}
