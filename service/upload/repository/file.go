package repository

import (
	"context"
	"fmt"
)

func FindUploadFile(ctx context.Context, fileID uint64) (UploadFile, error) {
	fileModel, err := ResolveFileModel()
	if err != nil {
		return UploadFile{}, err
	}

	row := fileModel.FindMap(ctx, map[string]any{"id": fileID})
	if len(row) == 0 {
		return UploadFile{}, fmt.Errorf("上传文件不存在")
	}

	record := NormalizeUploadFileRow(row)
	if record.StorageID != 0 {
		record.Storage, err = FindUploadStorage(ctx, record.StorageID)
		if err != nil {
			return UploadFile{}, err
		}
	}
	if err := HydrateUploadFile(ctx, &record); err != nil {
		return UploadFile{}, err
	}
	return record, nil
}

func FindUploadFileByPath(ctx context.Context, path string) *UploadFile {
	fileModel, err := ResolveFileModel()
	if err != nil {
		return nil
	}
	row := fileModel.FindMap(ctx, map[string]any{"path": path})
	if len(row) == 0 {
		return nil
	}

	record := NormalizeUploadFileRow(row)
	_ = HydrateUploadFile(ctx, &record)
	return &record
}

func HydrateUploadFile(ctx context.Context, record *UploadFile) error {
	if record == nil {
		return nil
	}
	if record.StorageID != 0 && record.Storage.ID == 0 {
		storageRecord, err := FindUploadStorage(ctx, record.StorageID)
		if err != nil {
			return err
		}
		record.Storage = storageRecord
	}
	if err := RepairUploadFileBizFromPath(ctx, record); err != nil {
		return err
	}
	if record.BizID == 0 {
		return nil
	}

	bizRecord, err := FindUploadBizByID(ctx, record.BizID)
	if err != nil {
		return err
	}
	record.BizKey = bizRecord.Key
	record.BizName = bizRecord.Name
	return nil
}
