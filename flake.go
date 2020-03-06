package main

import (
	"log"
	"os"

	"github.com/cnwinds/flake/server"
	cli "github.com/urfave/cli/v2"
)

func main() {

	app := &cli.App{
		Name: "flake",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "listen",
				Value: "127.0.0.1:10001",
				Usage: "listen address:port",
			},
			&cli.StringFlag{
				Name:  "etcdkeyprefix",
				Value: "/flake/",
				Usage: "etcd key path prefix",
			},
			&cli.StringSliceFlag{
				Name:  "etcdhosts",
				Value: cli.NewStringSlice("http://127.0.0.1:32379"),
				Usage: "etcd hosts",
			},
		},
		Action: func(c *cli.Context) error {
			cfg := server.Config{
				Endpoints:     c.StringSlice("etcdhosts"),
				ListenAddress: c.String("listen"),
				Prefix:        c.String("etcdkeyprefix"),
			}
			_, err := server.StartServer(&cfg)
			if err != nil {
				log.Fatal(err)
			}
			return err
		},
	}

	app.Run(os.Args)
}
