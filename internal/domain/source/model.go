package source

import "time"

type Kind string

const (
	KindRSS Kind = "rss"
	KindAPI Kind = "api"
)

func (k Kind) Valid() bool { return k == KindRSS || k == KindAPI }

type Source struct {
	ID            string
	Name          string
	Kind          Kind
	URL           string
	IsActive      bool
	LastFetchedAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
