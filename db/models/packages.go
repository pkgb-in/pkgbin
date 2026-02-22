package models

import (
	"time"
)

type Package struct {
	ID        int64     `db:"id"`
	Name      string    `db:"name"`
	CacheHit  int64     `db:"cache_hit"`
	CacheMiss int64     `db:"cache_miss"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}
