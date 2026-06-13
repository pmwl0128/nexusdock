package artifacts

import (
	"context"
	"time"
)

type FetchRepository interface {
	CreateFetch(context.Context, FetchJob) error
	ListFetches(context.Context, int) ([]FetchJob, error)
	GetFetch(context.Context, string) (FetchJob, error)
	SetFetchCommand(context.Context, string, string, FetchStatus, time.Time) (FetchJob, error)
	ClaimFetchUpload(context.Context, string, string, string, time.Time) (FetchJob, error)
	AbortFetchUpload(context.Context, string, time.Time) error
	CompleteFetchUpload(context.Context, FetchJob) error
	SetFetchResult(context.Context, string, string, FetchResultRequest, time.Time) (FetchJob, error)
	MarkFetchMounted(context.Context, string, string, time.Time) (FetchJob, error)
	ExpireFetches(context.Context, time.Time) ([]FetchJob, error)
}
