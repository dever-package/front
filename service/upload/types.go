package upload

import (
	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

const (
	uploadSessionPending   = "pending"
	uploadSessionUploading = "uploading"
	uploadSessionComplete  = "complete"
	uploadSizeMBUnit       = int64(1024 * 1024)
)

type uploadInitInput struct {
	RuleID     uint64 `json:"rule_id"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Mime       string `json:"mime"`
	Hash       string `json:"hash"`
	Kind       string `json:"kind"`
	BizKey     string `json:"biz_key"`
	BizName    string `json:"biz_name"`
	CategoryID uint64 `json:"category_id"`
}

type uploadCompleteInput struct {
	SessionID uint64 `json:"session_id"`
}

type resolvedUploadStorage = uploadrepo.UploadStorage
type resolvedUploadAcceptType = uploadrepo.UploadAcceptType
type resolvedUploadBiz = uploadrepo.UploadBiz
type resolvedUploadRule = uploadrepo.UploadRule
type resolvedUploadSession = uploadrepo.UploadSession
type resolvedUploadFile = uploadrepo.UploadFile
