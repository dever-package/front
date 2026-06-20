package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shemic/dever/util"
)

func FindUploadSession(ctx context.Context, sessionID uint64) (UploadSession, error) {
	sessionModel, err := ResolveSessionModel()
	if err != nil {
		return UploadSession{}, err
	}

	row := sessionModel.FindMap(ctx, map[string]any{"id": sessionID})
	if len(row) == 0 {
		return UploadSession{}, fmt.Errorf("上传会话不存在")
	}

	record := UploadSession{
		ID:               util.ToUint64(row["id"]),
		RuleID:           util.ToUint64(row["rule_id"]),
		StorageID:        util.ToUint64(row["storage_id"]),
		Kind:             ResolveKind(util.ToStringTrimmed(row["kind"]), util.ToStringTrimmed(row["name"]), util.ToStringTrimmed(row["mime"])),
		BizID:            util.ToUint64(row["biz_id"]),
		CategoryID:       util.ToUint64(row["category_id"]),
		Name:             util.ToStringTrimmed(row["name"]),
		Ext:              util.ToStringTrimmed(row["ext"]),
		Mime:             util.ToStringTrimmed(row["mime"]),
		Size:             util.ToInt64(row["size"]),
		Hash:             NormalizeHash(row["hash"]),
		Token:            util.ToStringTrimmed(row["token"]),
		ObjectKey:        util.ToStringTrimmed(row["object_key"]),
		ChunkSize:        util.ToInt64(row["chunk_size"]),
		ChunkTotal:       int(util.ToInt64(row["chunk_total"])),
		UploadedParts:    util.ToStringTrimmed(row["uploaded_parts"]),
		ProviderUploadID: util.ToStringTrimmed(row["provider_upload_id"]),
		Status:           util.ToStringTrimmed(row["status"]),
		ExpiredAt:        parseUploadSessionTime(row["expired_at"]),
	}
	if record.BizID == 0 {
		return record, nil
	}

	bizRecord, err := FindUploadBizByID(ctx, record.BizID)
	if err != nil {
		return UploadSession{}, err
	}
	record.BizKey = bizRecord.Key
	record.BizName = bizRecord.Name
	return record, nil
}

func UpdateUploadSession(ctx context.Context, sessionID uint64, updates map[string]any) error {
	sessionModel, err := ResolveSessionModel()
	if err != nil {
		return err
	}
	sessionModel.Update(ctx, map[string]any{"id": sessionID}, updates)
	return nil
}

func parseUploadSessionTime(value any) time.Time {
	switch current := value.(type) {
	case time.Time:
		return current
	case *time.Time:
		if current != nil {
			return *current
		}
	case string:
		text := strings.TrimSpace(current)
		if text == "" {
			return time.Time{}
		}
		for _, layout := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		} {
			parsed, err := time.ParseInLocation(layout, text, time.Local)
			if err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}
