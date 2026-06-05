package tasks

import "context"

// Repository is intentionally owned by T8. T1 can provide a SQLite adapter
// and transaction boundary without changing the Task domain service.
type Repository interface {
	CreateOrGet(context.Context, Task, string, Activity) (Task, bool, error)
	Get(context.Context, string) (Task, error)
	List(context.Context, Filter) (Page, error)
	Update(context.Context, Task, int64, Activity) (Task, error)
	Activities(context.Context, string) ([]Activity, error)
	GetIdempotency(context.Context, string, string) (Task, bool, error)
	PutIdempotency(context.Context, string, string, Task) error
}

type Authorizer interface {
	Can(context.Context, Actor, string, Task) bool
}

type AuthorizerFunc func(context.Context, Actor, string, Task) bool

func (f AuthorizerFunc) Can(ctx context.Context, actor Actor, action string, task Task) bool {
	return f(ctx, actor, action, task)
}

func AllowAllAuthorizer() Authorizer {
	return AuthorizerFunc(func(context.Context, Actor, string, Task) bool { return true })
}

type AuditRecord struct {
	Actor      Actor
	Action     string
	ObjectType string
	ObjectID   string
	Result     string
	Risk       string
	Metadata   map[string]any
}

type AuditSink interface {
	Record(context.Context, AuditRecord) error
}

type AuditSinkFunc func(context.Context, AuditRecord) error

func (f AuditSinkFunc) Record(ctx context.Context, record AuditRecord) error {
	return f(ctx, record)
}

type Event struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

type EventSink interface {
	Publish(context.Context, Event) error
}

type EventSinkFunc func(context.Context, Event) error

func (f EventSinkFunc) Publish(ctx context.Context, event Event) error {
	return f(ctx, event)
}

type discardAudit struct{}

func (discardAudit) Record(context.Context, AuditRecord) error { return nil }

type discardEvents struct{}

func (discardEvents) Publish(context.Context, Event) error { return nil }
