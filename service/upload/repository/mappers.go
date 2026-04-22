package repository

import "github.com/shemic/dever/util"

func mapUploadBiz(row map[string]any) UploadBiz {
	return UploadBiz{
		ID:   util.ToUint64(row["id"]),
		Key:  NormalizeBizKey(row["key"]),
		Name: NormalizeBizName(row["name"]),
	}
}

func mapUploadStorage(row map[string]any) UploadStorage {
	return UploadStorage{
		ID:         util.ToUint64(row["id"]),
		Name:       util.ToStringTrimmed(row["name"]),
		Type:       util.ToStringTrimmed(row["type"]),
		AccessKey:  util.ToStringTrimmed(row["access_key"]),
		SecretKey:  util.ToStringTrimmed(row["secret_key"]),
		Bucket:     util.ToStringTrimmed(row["bucket"]),
		Domain:     util.ToStringTrimmed(row["domain"]),
		UploadHost: util.ToStringTrimmed(row["upload_host"]),
		TokenTTL:   util.ToInt64(row["token_ttl"]),
	}
}

func mapUploadAcceptType(row map[string]any) UploadAcceptType {
	return UploadAcceptType{
		ID:     util.ToUint64(row["id"]),
		Name:   util.ToStringTrimmed(row["name"]),
		Accept: util.ToStringTrimmed(row["accept"]),
	}
}

func NormalizeUploadFileRow(row map[string]any) UploadFile {
	return UploadFile{
		ID:         util.ToUint64(row["id"]),
		RuleID:     util.ToUint64(row["rule_id"]),
		StorageID:  util.ToUint64(row["storage_id"]),
		Kind:       ResolveKind(util.ToStringTrimmed(row["kind"]), util.ToStringTrimmed(row["name"]), util.ToStringTrimmed(row["mime"])),
		BizID:      util.ToUint64(row["biz_id"]),
		CategoryID: util.ToUint64(row["category_id"]),
		Name:       util.ToStringTrimmed(row["name"]),
		Ext:        util.ToStringTrimmed(row["ext"]),
		Mime:       util.ToStringTrimmed(row["mime"]),
		Size:       util.ToInt64(row["size"]),
		Hash:       NormalizeHash(row["hash"]),
		Path:       util.ToStringTrimmed(row["path"]),
		CreatedAt:  util.ToStringTrimmed(row["created_at"]),
	}
}
