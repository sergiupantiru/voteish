package repository

import (
	"log"
	"time"

	"github.com/sergiupantiru/voteish/internal/voting"

	"github.com/patrickmn/go-cache"
)

type Repository struct {
	cache *cache.Cache
}

func NewRepository() *Repository {

	return &Repository{
		cache: cache.New(30*time.Minute, 40*time.Minute),
	}
}

func (r *Repository) AddSession(session *voting.VotingSession) {
	r.cache.Add(session.SessionId, session, cache.DefaultExpiration)
}

func (r *Repository) Get(sessionId string) (*voting.VotingSession, bool) {
	item, found := r.cache.Get(sessionId)
	return item.(*voting.VotingSession), found
}

func (r *Repository) Remove(sessionId string) {
	r.cache.Delete(sessionId)
	log.Printf("Active session %d", r.cache.ItemCount())
}
