package storage

import (
	"context"
	"errors"
	"time"
	"uuid"
)

// ErrBlobMetadataConflict indicates that immutable database metadata disagrees with stored content.
var ErrBlobMetadataConflict = errors.New("blob metadata conflicts with an existing digest")

// ReferenceRegistry atomically registers tenant ownership and unique-byte accounting.
type ReferenceRegistry interface {
	AddBlobReference(context.Context, uuid.UUID, Blob, string, time.Time) error
}
