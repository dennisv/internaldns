package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	etcd "go.etcd.io/etcd/clientv3"
)

type host struct {
	IPAddress string
}
type config struct {
	EtcdEndpoint string `toml:"etcd_endpoint"`
	Hosts        map[string]host
}

var conf config

func reverse(ss []string) {
	last := len(ss) - 1
	for i := 0; i < len(ss)/2; i++ {
		ss[i], ss[last-i] = ss[last-i], ss[i]
	}
}

func formatHostname(hostname string) string {
	s := strings.Split(hostname, ".")
	fmt.Printf("%#v\n", s)
	reverse(s)
	fmt.Printf("%#v\n", s)
	return fmt.Sprintf("/internaldns/%s", strings.Join(s, "/"))
}

func getIPAddress(hostname string) (string, error) {
	match := ""
	for suffix, _ := range conf.Hosts {
		if strings.HasSuffix(hostname, suffix) && len(suffix) > len(match) {
			match = suffix
		}
	}
	if host, ok := conf.Hosts[match]; ok {
		return host.IPAddress, nil
	}
	return "", errors.New("No host suffix found matching hostname")
}

func handleEvent(msg events.Message, c *etcd.Client) {
	if hostname, ok := msg.Actor.Attributes["internaldns.host"]; ok {
		ipaddress, err := getIPAddress(hostname)
		if err != nil {
			return
		}
		fmt.Printf("found host suffix match %s -> %s\n", hostname, ipaddress)
		fmt.Printf("%#v\n", msg)
		if msg.Action == "start" || msg.Action == "create" {
			log.Println("add dns")
			val := fmt.Sprintf("{\"host\":\"%s\"}", ipaddress)
			resp, err := c.Put(context.Background(), formatHostname(hostname), val)
			if err != nil {
				log.Println(err)
			} else {
				log.Printf("Added %s as %s (%q)\n", msg.ID, hostname, resp)
			}
		}
		if msg.Action == "stop" || msg.Action == "die" || msg.Action == "destroy" {
			log.Println("delete dns")
			resp, err := c.Delete(context.Background(), formatHostname(hostname))
			if err != nil {
				log.Println(err)
			} else {
				log.Printf("Deleted %s as %s (%q)\n", msg.ID, hostname, resp)
			}
		}
	}
}

func main() {
	if _, err := toml.DecodeFile("config.toml", &conf); err != nil {
		panic(err)
	}
	ctx := context.Background()

	cfg := etcd.Config{
		Endpoints:   []string{conf.EtcdEndpoint},
		DialTimeout: 5 * time.Second,
	}
	c, err := etcd.New(cfg)
	if err != nil {
		panic(err)
	}
	defer c.Close()

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}

	cli.NegotiateAPIVersion(ctx)

	filterArgs := filters.NewArgs()
	filterArgs.Add("event", "start")
	filterArgs.Add("event", "create")
	filterArgs.Add("event", "stop")
	filterArgs.Add("event", "die")
	filterArgs.Add("event", "destroy")

	eventsOptions := types.EventsOptions{
		Filters: filterArgs,
	}

	messageChan, errorChan := cli.Events(ctx, eventsOptions)
	for {
		select {
		case msg := <-messageChan:
			fmt.Printf("%#v\n", c)
			handleEvent(msg, c)
		case err := <-errorChan:
			log.Println(err)
			messageChan, errorChan = cli.Events(ctx, eventsOptions)
		}
	}
}
