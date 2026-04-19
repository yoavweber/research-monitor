package source

import "time"

type Response struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Kind          Kind       `json:"kind"`
	URL           string     `json:"url"`
	IsActive      bool       `json:"is_active"`
	LastFetchedAt *time.Time `json:"last_fetched_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func ToResponse(s *Source) Response {
	return Response{
		ID:            s.ID,
		Name:          s.Name,
		Kind:          s.Kind,
		URL:           s.URL,
		IsActive:      s.IsActive,
		LastFetchedAt: s.LastFetchedAt,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

func ToResponseList(xs []Source) []Response {
	out := make([]Response, len(xs))
	for i := range xs {
		out[i] = ToResponse(&xs[i])
	}
	return out
}
