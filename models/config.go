package models

import (
	"time"

	"github.com/BurntSushi/toml"
)

const configPath = "config/config.toml"

type duration time.Duration

// Config struct
type Config struct {
	RedisConfig Redis `toml:"Redis"`
}

func (d *duration) UnmarshalText(text []byte) error {
	temp, err := time.ParseDuration(string(text))
	*d = duration(temp)
	return err
}

// ServerOpt struct
type Redis struct {
	Host         string
	Password     string
	ReadTimeout  duration
	WriteTimeout duration
	DialTimeout  duration
}

// LoadConfig from path
func LoadConfig(c *Config) error {
	_, err := toml.DecodeFile(configPath, c)
	if err != nil {
		return err
	}
	return nil
}
