package mycache

import (
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/os/gcache"
)

type RedisConfig struct {
	Host     string `mapstructure:"host" json:"host" yaml:"host"`
	User     string `mapstructure:"user" json:"user" yaml:"user"`
	Password string `mapstructure:"password" json:"password" yaml:"password"`
	Port     string `mapstructure:"port" json:"port" yaml:"port"`
	Db       int    `mapstructure:"db" json:"db" yaml:"db"`
	Prefix   string `mapstructure:"prefix" json:"prefix" yaml:"prefix"`
}

var cacheIns *gcache.Cache

func Init(conf RedisConfig) error {
	redisConfig := &gredis.Config{
		Address: conf.Host + ":" + conf.Port,
		Db:      conf.Db,
		Pass:    conf.Password,
		User:    conf.User,
	}
	redisClient, err := gredis.New(redisConfig)
	if err != nil {
		return err
	}

	cacheIns = gcache.New()
	cacheIns.SetAdapter(gcache.NewAdapterRedis(redisClient))
	return nil
}

func Cache() *gcache.Cache {
	return cacheIns
}
