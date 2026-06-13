package artifacts

import (
	"context"
	"time"
)

type Repository interface {
	Create(context.Context, Artifact, []Delivery) error
	ListArtifacts(context.Context, int) ([]Artifact, error)
	GetArtifact(context.Context, string) (Artifact, error)
	ListDeliveries(context.Context, string) ([]Delivery, error)
	GetDelivery(context.Context, string) (Delivery, error)
	ClaimUpload(context.Context, string, string, time.Time) (Artifact, error)
	AbortUpload(context.Context, string, time.Time) error
	FinalizeUpload(context.Context, Artifact, []Delivery) error
	SetDeliveryQueued(context.Context, string, string, string, time.Time, DeliveryStatus, time.Time) (Delivery, error)
	SetDeliveryResult(context.Context, string, DeliveryResultRequest, time.Time) (Delivery, error)
	MarkDeliveryDownloading(context.Context, string, time.Time) (Delivery, error)
	ExpireBefore(context.Context, time.Time) ([]Artifact, error)
	MarkArtifactDeleted(context.Context, string, time.Time) error
}
