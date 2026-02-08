package redis

import "time"

type Options struct {
	Addr     string `json:"addr"`
	Username string `json:"username"`
	Password string `json:"password"`
	DB       int    `json:"db"`

	DefaultTTL time.Duration `json:"default_ttl"`
	Prefix     string        `json:"prefix"`
}
