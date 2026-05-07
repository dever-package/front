package repository

import (
	frontmodel "my/package/front/model"
)

type UploadStorage = frontmodel.UploadStorage
type UploadAcceptType = frontmodel.UploadAcceptType
type UploadBiz = frontmodel.UploadFileBiz

type UploadRule struct {
	ID            uint64
	Name          string
	StorageID     uint64
	AcceptTypeID  uint64
	AcceptTypeIDs []uint64
	Transport     string
	Accept        string
	ChunkSizeMB   int64
	MaxSizeMB     int64
	Status        int
	Storage       UploadStorage
	AcceptType    UploadAcceptType
	AcceptTypes   []UploadAcceptType
}

type UploadSession struct {
	ID               uint64
	RuleID           uint64
	StorageID        uint64
	Kind             string
	BizID            uint64
	BizKey           string
	BizName          string
	CategoryID       uint64
	Name             string
	Ext              string
	Mime             string
	Size             int64
	Hash             string
	ObjectKey        string
	ChunkSize        int64
	ChunkTotal       int
	UploadedParts    string
	ProviderUploadID string
	Status           string
}

type UploadFile struct {
	ID         uint64
	RuleID     uint64
	StorageID  uint64
	Kind       string
	BizID      uint64
	BizKey     string
	BizName    string
	CategoryID uint64
	Name       string
	Ext        string
	Mime       string
	Size       int64
	Hash       string
	Path       string
	CreatedAt  string
	Storage    UploadStorage
}
