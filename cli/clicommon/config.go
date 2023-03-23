package clicommon

import (
	"gopkg.in/urfave/cli.v1"
)

type Config struct {
	LogDir      string
	DebugMode   bool
	VerboseMode bool
	NatsUrl     string
}

func CreateConfig(c *cli.Context) Config {
	return Config{
		LogDir:      c.GlobalString("log-dir"),
		DebugMode:   c.GlobalBool("debug"),
		VerboseMode: c.GlobalBool("verbose"),
		NatsUrl:     c.GlobalString("nats_url"),
	}
}
