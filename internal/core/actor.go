package core

type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorAgent  ActorType = "agent"
	ActorDevice ActorType = "device"
	ActorSystem ActorType = "system"
)

type Actor struct {
	Type ActorType `json:"type"`
	ID   string    `json:"id"`
}

func (a Actor) Valid() bool {
	if a.ID == "" {
		return false
	}
	switch a.Type {
	case ActorUser, ActorAgent, ActorDevice, ActorSystem:
		return true
	default:
		return false
	}
}
