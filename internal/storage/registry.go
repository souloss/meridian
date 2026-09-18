package storage

import (
	"context"
	"errors"
	"time"
	"uuid"
)

// ErrBlobMetadataConflict 表示不可变的数据库元数据与已存储内容不一致。
var ErrBlobMetadataConflict = errors.New("blob metadata conflicts with an existing digest")

// ReferenceRegistry 原子地登记租户所有权与唯一字节记账。
type ReferenceRegistry interface {
	AddBlobReference(context.Context, uuid.UUID, Blob, string, time.Time) error
}
